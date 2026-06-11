package cleanup

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/iamrajjoshi/willow/internal/config"
	"github.com/iamrajjoshi/willow/internal/gh"
	"github.com/iamrajjoshi/willow/internal/git"
	"github.com/iamrajjoshi/willow/internal/stack"
	"github.com/iamrajjoshi/willow/internal/worktree"
)

type Reason string

const (
	ReasonMergedPR     Reason = "merged-pr"
	ReasonGoneUpstream Reason = "gone-upstream"
)

type Candidate struct {
	RepoName        string
	BareDir         string
	Branch          string
	Head            string
	Path            string
	WtDirName       string
	ExpectedBaseRef string
	Reasons         []Reason
}

type Skip struct {
	Candidate Candidate
	Reason    string
}

type ScanOptions struct {
	RefreshPRState bool
	Verbose        bool
}

type UpstreamStatus struct {
	Upstream string
	Gone     bool
}

func ScanRepo(repoName, bareDir string, opts ScanOptions) ([]Candidate, error) {
	repoGit := &git.Git{Dir: bareDir, Verbose: opts.Verbose}
	wts, err := worktree.List(repoGit)
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}
	return CandidatesForWorktrees(repoName, bareDir, wts, opts)
}

func CandidatesForWorktrees(repoName, bareDir string, wts []worktree.Worktree, opts ScanOptions) ([]Candidate, error) {
	repoGit := &git.Git{Dir: bareDir, Verbose: opts.Verbose}
	cfg := config.Load(bareDir)
	st := stack.Load(bareDir)
	baseBranch := repoGit.ResolveBaseBranch(cfg.BaseBranch)

	branchHeads := make(map[string]string, len(wts))
	branchBases := make(map[string]string, len(wts))
	branchNames := make([]string, 0, len(wts))
	repoDir := ""
	for _, wt := range wts {
		if wt.IsBare || wt.Detached || wt.Branch == "" {
			continue
		}
		if repoDir == "" {
			repoDir = wt.Path
		}
		branchHeads[wt.Branch] = wt.Head
		branchNames = append(branchNames, wt.Branch)
		if parent := st.Parent(wt.Branch); parent != "" {
			branchBases[wt.Branch] = parent
		}
	}
	if len(branchNames) == 0 {
		return nil, nil
	}

	var mergedSet map[string]bool
	if opts.RefreshPRState {
		mergedSet = gh.MergedWorktreeSet(repoDir, baseBranch, branchHeads, branchBases)
	} else {
		mergedSet = gh.CachedMergedWorktreeSet(repoDir, baseBranch, branchHeads, branchBases)
	}

	upstreams, err := UpstreamStatuses(repoGit, branchNames)
	if err != nil {
		return nil, err
	}

	var candidates []Candidate
	for _, wt := range wts {
		if wt.IsBare || wt.Detached || wt.Branch == "" {
			continue
		}
		var reasons []Reason
		if mergedSet[wt.Branch] {
			reasons = append(reasons, ReasonMergedPR)
		}
		if upstreams[wt.Branch].Gone {
			reasons = append(reasons, ReasonGoneUpstream)
		}
		if len(reasons) == 0 {
			continue
		}
		candidates = append(candidates, Candidate{
			RepoName:        repoName,
			BareDir:         bareDir,
			Branch:          wt.Branch,
			Head:            wt.Head,
			Path:            wt.Path,
			WtDirName:       filepath.Base(wt.Path),
			ExpectedBaseRef: ExpectedBaseRef(repoGit, st, baseBranch, wt.Branch),
			Reasons:         reasons,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].RepoName != candidates[j].RepoName {
			return candidates[i].RepoName < candidates[j].RepoName
		}
		return candidates[i].Branch < candidates[j].Branch
	})
	return candidates, nil
}

func UpstreamStatuses(g *git.Git, branches []string) (map[string]UpstreamStatus, error) {
	statuses := make(map[string]UpstreamStatus, len(branches))
	if len(branches) == 0 {
		return statuses, nil
	}

	args := []string{"for-each-ref", "--format=%(refname:short)%00%(upstream:short)%00%(upstream:track)"}
	for _, branch := range branches {
		if branch != "" {
			args = append(args, "refs/heads/"+branch)
		}
	}
	out, err := g.Run(args...)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect branch upstreams: %w", err)
	}
	if out == "" {
		return statuses, nil
	}

	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) < 3 {
			continue
		}
		branch := strings.TrimSpace(fields[0])
		upstream := strings.TrimSpace(fields[1])
		track := strings.TrimSpace(fields[2])
		if branch == "" {
			continue
		}
		statuses[branch] = UpstreamStatus{
			Upstream: upstream,
			Gone:     upstream != "" && strings.Contains(track, "[gone]"),
		}
	}
	return statuses, nil
}

func ExpectedBaseRef(repoGit *git.Git, st *stack.Stack, baseBranch, branch string) string {
	if st != nil {
		if parent := st.Parent(branch); parent != "" {
			if st.IsTracked(parent) && repoGit.LocalBranchExists(parent) {
				return "refs/heads/" + parent
			}
			return "refs/remotes/origin/" + parent
		}
	}
	if baseBranch == "" {
		return ""
	}
	return "refs/remotes/origin/" + baseBranch
}

func FilterSafe(candidates []Candidate) ([]Candidate, []Skip, error) {
	stacks := make(map[string]*stack.Stack)
	var safe []Candidate
	var skipped []Skip

	for _, candidate := range candidates {
		st, ok := stacks[candidate.BareDir]
		if !ok {
			st = stack.Load(candidate.BareDir)
			stacks[candidate.BareDir] = st
		}
		reason, err := SkipReason(candidate, st)
		if err != nil {
			return nil, nil, err
		}
		if reason != "" {
			skipped = append(skipped, Skip{Candidate: candidate, Reason: reason})
			continue
		}
		safe = append(safe, candidate)
	}
	return safe, skipped, nil
}

func SkipReason(candidate Candidate, st *stack.Stack) (string, error) {
	var children []string
	if st != nil {
		children = st.Children(candidate.Branch)
	}

	wtGit := &git.Git{Dir: candidate.Path}
	dirty, err := wtGit.IsDirty()
	if err != nil {
		return "", err
	}

	reachable := true
	if !candidate.HasReason(ReasonMergedPR) {
		reachable = false
		if candidate.ExpectedBaseRef == "" {
			return skipReasonFromState(children, dirty, false, "base ref missing"), nil
		}

		repoGit := &git.Git{Dir: candidate.BareDir}
		if !refExists(repoGit, candidate.ExpectedBaseRef) {
			return skipReasonFromState(children, dirty, false, "base ref missing: "+shortRef(candidate.ExpectedBaseRef)), nil
		}
		branchRef := "refs/heads/" + candidate.Branch
		if !refExists(repoGit, branchRef) {
			return skipReasonFromState(children, dirty, false, "branch ref missing: "+candidate.Branch), nil
		}
		reachable = isAncestor(repoGit, branchRef, candidate.ExpectedBaseRef)
	}

	return skipReasonFromState(children, dirty, reachable, "not merged into "+shortRef(candidate.ExpectedBaseRef)), nil
}

func skipReasonFromState(children []string, dirty, reachable bool, reachabilityReason string) string {
	var reasons []string
	if len(children) > 0 {
		reasons = append(reasons, "stacked children: "+strings.Join(children, ", "))
	}
	if dirty {
		reasons = append(reasons, "uncommitted changes")
	}
	if !reachable {
		reasons = append(reasons, reachabilityReason)
	}
	return strings.Join(reasons, "; ")
}

func refExists(g *git.Git, ref string) bool {
	_, err := g.Run("rev-parse", "--verify", ref+"^{commit}")
	return err == nil
}

func isAncestor(g *git.Git, ancestor, descendant string) bool {
	_, err := g.Run("merge-base", "--is-ancestor", ancestor, descendant)
	return err == nil
}

func (c Candidate) HasReason(reason Reason) bool {
	for _, r := range c.Reasons {
		if r == reason {
			return true
		}
	}
	return false
}

func (c Candidate) ReasonString() string {
	return ReasonsString(c.Reasons)
}

func ReasonsString(reasons []Reason) string {
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		parts = append(parts, string(reason))
	}
	return strings.Join(parts, ", ")
}

func Label(candidate Candidate, multiRepo bool) string {
	if multiRepo {
		return candidate.RepoName + "/" + candidate.Branch
	}
	return candidate.Branch
}

func HasMultipleRepos(candidates []Candidate) bool {
	repos := make(map[string]bool)
	for _, candidate := range candidates {
		repos[candidate.RepoName] = true
		if len(repos) > 1 {
			return true
		}
	}
	return false
}

func shortRef(ref string) string {
	ref = strings.TrimPrefix(ref, "refs/heads/")
	ref = strings.TrimPrefix(ref, "refs/remotes/")
	return ref
}
