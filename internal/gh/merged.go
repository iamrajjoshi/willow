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
	mergedWorktreeCacheTTL  = 30 * time.Second
	mergedWorktreeBatchSize = 20
	mergedWorktreeCacheDir  = "merged-worktrees"
	mergedWorktreeSearchCap = 100
	detachedBranchName      = "(detached)"
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
	Entries map[string]mergedWorktreeCacheEntry `json:"entries"`
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

// MergedWorktreeSet returns branches whose worktrees are safe to treat as
// merged based on GitHub PR state. It is designed to augment git ancestry-
// based merged detection and silently falls back to an empty set when `gh`
// is unavailable or GitHub lookup fails.
func MergedWorktreeSet(dir, base string, branchHeads map[string]string) map[string]bool {
	set := make(map[string]bool)
	if dir == "" || len(branchHeads) == 0 || !mergedWorktreeCLI() {
		return set
	}

	now := mergedWorktreeNow()
	cachePath := mergedWorktreeCachePath(dir, base)
	cache := loadMergedWorktreeCache(cachePath)

	eligibleHeads := make(map[string]string, len(branchHeads))
	var pending []string
	for branch, head := range branchHeads {
		if branch == "" || branch == base || branch == detachedBranchName || head == "" {
			continue
		}

		eligibleHeads[branch] = head
		entry, ok := cache.Entries[branch]
		if ok && now.Sub(entry.CheckedAt) < mergedWorktreeCacheTTL {
			if entry.isMerged(base, head) {
				set[branch] = true
			}
			continue
		}

		pending = append(pending, branch)
	}

	if len(pending) == 0 {
		return set
	}

	sort.Strings(pending)
	updated, _ := refreshMergedWorktreeCache(cache, dir, pending, now)
	_ = saveMergedWorktreeCache(cachePath, updated)

	for branch, head := range eligibleHeads {
		entry, ok := updated.Entries[branch]
		if !ok || now.Sub(entry.CheckedAt) >= mergedWorktreeCacheTTL {
			continue
		}
		if entry.isMerged(base, head) {
			set[branch] = true
		}
	}

	return set
}

func (e mergedWorktreeCacheEntry) isMerged(base, head string) bool {
	return e.Found &&
		e.State == "MERGED" &&
		e.BaseRefName == base &&
		e.HeadRefOID == head
}

func refreshMergedWorktreeCache(cache mergedWorktreeCache, dir string, branches []string, now time.Time) (mergedWorktreeCache, error) {
	updated := cache.clone()

	for start := 0; start < len(branches); start += mergedWorktreeBatchSize {
		end := start + mergedWorktreeBatchSize
		if end > len(branches) {
			end = len(branches)
		}

		batch := branches[start:end]
		prs, err := mergedWorktreeSearchPRs(dir, batch)
		if err != nil {
			return updated, err
		}

		latest := latestPRsByBranch(batch, prs)
		for _, branch := range batch {
			entry := mergedWorktreeCacheEntry{CheckedAt: now}
			if pr := latest[branch]; pr != nil {
				entry.Found = true
				entry.Number = pr.Number
				entry.State = pr.State
				entry.BaseRefName = pr.BaseRefName
				entry.HeadRefOID = pr.HeadRefOID
				entry.MergedAt = pr.MergedAt
			}
			updated.Entries[branch] = entry
		}
	}

	return updated, nil
}

func latestPRsByBranch(branches []string, prs []*PRInfo) map[string]*PRInfo {
	branchSet := make(map[string]bool, len(branches))
	for _, branch := range branches {
		branchSet[branch] = true
	}

	latest := make(map[string]*PRInfo, len(branches))
	for _, pr := range prs {
		if pr == nil || !branchSet[pr.Branch] {
			continue
		}
		if current := latest[pr.Branch]; current == nil || pr.Number > current.Number {
			latest[pr.Branch] = pr
		}
	}

	return latest
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
	sum := sha256.Sum256([]byte(mergedWorktreeRepoKey(dir) + "\x00" + base))
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
		return mergedWorktreeCache{Entries: make(map[string]mergedWorktreeCacheEntry)}
	}

	var cache mergedWorktreeCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return mergedWorktreeCache{Entries: make(map[string]mergedWorktreeCacheEntry)}
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
		Entries: make(map[string]mergedWorktreeCacheEntry, len(c.Entries)),
	}
	for branch, entry := range c.Entries {
		cloned.Entries[branch] = entry
	}
	return cloned
}
