package gh

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/config"
)

const (
	mergedWorktreeCacheVersion = 2
	mergedWorktreeCacheTTL     = 30 * time.Second
	mergedWorktreeBatchSize    = 20
	mergedWorktreeCacheDir     = "merged-worktrees"
	mergedWorktreeSearchCap    = 100
	detachedBranchName         = "(detached)"
)

type mergedWorktreeCacheEntry struct {
	CheckedAt   time.Time `json:"checked_at"`
	Found       bool      `json:"found"`
	Number      int       `json:"number,omitempty"`
	State       string    `json:"state,omitempty"`
	BaseRefName string    `json:"base_ref_name,omitempty"`
	HeadRefOID  string    `json:"head_ref_oid,omitempty"`
	MergedAt    string    `json:"merged_at,omitempty"`
}

type mergedWorktreeCache struct {
	Version int                                 `json:"version"`
	Entries map[string]mergedWorktreeCacheEntry `json:"entries"`
}

type mergeCandidate struct {
	branch string
	head   string
	base   string
}

var (
	mergedWorktreeNow = time.Now
	mergedWorktreeCLI = func() bool {
		_, err := exec.LookPath("gh")
		return err == nil
	}
	mergedWorktreeSearchPRs = searchPRsByBranches
	mergedWorktreeRepoKey   = repoCacheKey
)

// MergedWorktreeSet returns branches whose current worktree heads have exact
// merged PRs on GitHub. It deliberately does not use git ancestry: in Willow,
// `[merged]` is a PR lifecycle signal, not a reachability signal.
func MergedWorktreeSet(dir, defaultBase string, branchHeads, branchBases map[string]string) map[string]bool {
	return mergedWorktreeSet(dir, defaultBase, branchHeads, branchBases, true)
}

// CachedMergedWorktreeSet returns fresh merged branches from the on-disk
// GitHub lookup cache only. It never shells out to gh, so interactive pickers
// can use it without putting network-backed PR lookups on their initial render
// path. Stale positives are ignored rather than risk showing a false
// `[merged]` tag for a reused branch.
func CachedMergedWorktreeSet(dir, defaultBase string, branchHeads, branchBases map[string]string) map[string]bool {
	return mergedWorktreeSet(dir, defaultBase, branchHeads, branchBases, false)
}

func mergedWorktreeSet(dir, defaultBase string, branchHeads, branchBases map[string]string, refresh bool) map[string]bool {
	set := make(map[string]bool)
	candidates := mergedCandidates(defaultBase, branchHeads, branchBases)
	if dir == "" || len(candidates) == 0 {
		return set
	}

	now := mergedWorktreeNow()
	cachePath := mergedWorktreeCachePath(dir, defaultBase)
	cache := loadMergedWorktreeCache(cachePath)

	var pending []mergeCandidate
	for _, candidate := range candidates {
		entry, ok := cache.Entries[candidate.branch]
		if ok && entry.isFreshFor(candidate, now) {
			if entry.isMerged(candidate) {
				set[candidate.branch] = true
			}
			continue
		}

		pending = append(pending, candidate)
	}

	if !refresh || len(pending) == 0 || !mergedWorktreeCLI() {
		return set
	}

	sort.Slice(pending, func(i, j int) bool {
		return pending[i].branch < pending[j].branch
	})
	updated, _ := refreshMergedWorktreeCache(cache, dir, pending, now)
	_ = saveMergedWorktreeCache(cachePath, updated)

	for _, candidate := range candidates {
		entry, ok := updated.Entries[candidate.branch]
		if !ok || !entry.isFreshFor(candidate, now) {
			continue
		}
		if entry.isMerged(candidate) {
			set[candidate.branch] = true
		}
	}

	return set
}

func mergedCandidates(defaultBase string, branchHeads, branchBases map[string]string) []mergeCandidate {
	candidates := make([]mergeCandidate, 0, len(branchHeads))
	for branch, head := range branchHeads {
		base := defaultBase
		if branchBases != nil && branchBases[branch] != "" {
			base = branchBases[branch]
		}
		if branch == "" || branch == base || branch == detachedBranchName || head == "" || base == "" {
			continue
		}
		candidates = append(candidates, mergeCandidate{branch: branch, head: head, base: base})
	}
	return candidates
}

func (e mergedWorktreeCacheEntry) isFreshFor(candidate mergeCandidate, now time.Time) bool {
	return now.Sub(e.CheckedAt) < mergedWorktreeCacheTTL &&
		e.BaseRefName == candidate.base &&
		e.HeadRefOID == candidate.head
}

func (e mergedWorktreeCacheEntry) isMerged(candidate mergeCandidate) bool {
	return e.Found &&
		e.State == "MERGED" &&
		e.BaseRefName == candidate.base &&
		e.HeadRefOID == candidate.head
}

func refreshMergedWorktreeCache(cache mergedWorktreeCache, dir string, candidates []mergeCandidate, now time.Time) (mergedWorktreeCache, error) {
	updated := cache.clone()

	for start := 0; start < len(candidates); start += mergedWorktreeBatchSize {
		end := start + mergedWorktreeBatchSize
		if end > len(candidates) {
			end = len(candidates)
		}

		batch := candidates[start:end]
		prs, err := mergedWorktreeSearchPRs(dir, candidateBranches(batch))
		if err != nil {
			return updated, err
		}

		for _, candidate := range batch {
			entry := mergedWorktreeCacheEntry{
				CheckedAt:   now,
				BaseRefName: candidate.base,
				HeadRefOID:  candidate.head,
			}
			if pr := selectExactPR(candidate, prs); pr != nil {
				entry.Found = true
				entry.Number = pr.Number
				entry.State = pr.State
				entry.MergedAt = pr.MergedAt
			}
			updated.Entries[candidate.branch] = entry
		}
	}

	return updated, nil
}

func candidateBranches(candidates []mergeCandidate) []string {
	branches := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		branches = append(branches, candidate.branch)
	}
	return branches
}

func selectExactPR(candidate mergeCandidate, prs []*PRInfo) *PRInfo {
	var selected *PRInfo
	for _, pr := range prs {
		if pr == nil ||
			pr.Branch != candidate.branch ||
			pr.HeadRefOID != candidate.head ||
			pr.BaseRefName != candidate.base {
			continue
		}
		if betterExactPR(pr, selected) {
			selected = pr
		}
	}
	return selected
}

func betterExactPR(candidate, current *PRInfo) bool {
	if current == nil {
		return true
	}
	return candidate.Number > current.Number
}

func searchPRsByBranches(dir string, branches []string) ([]*PRInfo, error) {
	if len(branches) == 0 {
		return nil, nil
	}

	limit := mergedWorktreeSearchCap
	if perBatch := len(branches) * 5; perBatch > limit {
		limit = perBatch
	}

	args := []string{
		"pr", "list",
		"--search", buildMergedWorktreeSearch(branches),
		"--state", "all",
		"--json", prJSONFields,
		"--limit", fmt.Sprintf("%d", limit),
	}

	cmd := exec.Command("gh", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GH_PROMPT_DISABLED=1",
		"GH_NO_UPDATE_NOTIFIER=1",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return nil, fmt.Errorf("gh pr list failed: %s", msg)
		}
		return nil, fmt.Errorf("gh pr list failed: %w", err)
	}

	return parsePRListOutput(out)
}

func buildMergedWorktreeSearch(branches []string) string {
	terms := make([]string, 0, len(branches))
	for _, branch := range branches {
		terms = append(terms, headSearchTerm(branch))
	}
	return strings.Join(terms, " OR ")
}

func headSearchTerm(branch string) string {
	if strings.ContainsAny(branch, " \t\"") {
		return fmt.Sprintf(`head:"%s"`, strings.ReplaceAll(branch, `"`, `\"`))
	}
	return "head:" + branch
}

func mergedWorktreeCachePath(dir, base string) string {
	key := fmt.Sprintf("v%d\x00%s\x00%s", mergedWorktreeCacheVersion, mergedWorktreeRepoKey(dir), base)
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(config.WillowHome(), "cache", mergedWorktreeCacheDir, fmt.Sprintf("%x.json", sum[:]))
}

func repoCacheKey(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return filepath.Clean(dir)
	}

	key := strings.TrimSpace(string(out))
	if key == "" {
		return filepath.Clean(dir)
	}
	if !filepath.IsAbs(key) {
		key = filepath.Join(dir, key)
	}
	return filepath.Clean(key)
}

func loadMergedWorktreeCache(path string) mergedWorktreeCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return emptyMergedWorktreeCache()
	}

	var cache mergedWorktreeCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return emptyMergedWorktreeCache()
	}
	if cache.Version != mergedWorktreeCacheVersion {
		return emptyMergedWorktreeCache()
	}
	if cache.Entries == nil {
		cache.Entries = make(map[string]mergedWorktreeCacheEntry)
	}
	return cache
}

func saveMergedWorktreeCache(path string, cache mergedWorktreeCache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	cache.Version = mergedWorktreeCacheVersion
	data, err := json.Marshal(cache)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func (c mergedWorktreeCache) clone() mergedWorktreeCache {
	cloned := mergedWorktreeCache{
		Version: mergedWorktreeCacheVersion,
		Entries: make(map[string]mergedWorktreeCacheEntry, len(c.Entries)),
	}
	for branch, entry := range c.Entries {
		cloned.Entries[branch] = entry
	}
	return cloned
}

func emptyMergedWorktreeCache() mergedWorktreeCache {
	return mergedWorktreeCache{
		Version: mergedWorktreeCacheVersion,
		Entries: make(map[string]mergedWorktreeCacheEntry),
	}
}
