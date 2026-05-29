package worktree

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/iamrajjoshi/willow/internal/git"
)

const DetachedBranch = "(detached)"

type Worktree struct {
	Branch   string `json:"branch"`
	Path     string `json:"path"`
	Head     string `json:"head"`
	Detached bool   `json:"detached,omitempty"`
	IsBare   bool   `json:"-"`
}

func (wt Worktree) DirName() string {
	return filepath.Base(wt.Path)
}

func (wt Worktree) DisplayName() string {
	if !wt.Detached {
		return wt.Branch
	}
	if wt.Head == "" {
		return fmt.Sprintf("%s [detached]", wt.DirName())
	}
	return fmt.Sprintf("%s [detached %s]", wt.DirName(), ShortHead(wt.Head))
}

func (wt Worktree) MatchName() string {
	if wt.Detached {
		return wt.DirName()
	}
	return wt.Branch
}

func ShortHead(head string) string {
	if len(head) <= 7 {
		return head
	}
	return head[:7]
}

func List(g *git.Git) ([]Worktree, error) {
	if g.Dir != "" && !g.Verbose {
		if worktrees, ok := listFromGitMetadata(g.Dir); ok {
			return worktrees, nil
		}
	}

	out, err := g.Run("worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	return parsePorcelain(out), nil
}

func listFromGitMetadata(commonDir string) ([]Worktree, bool) {
	commonDir = filepath.Clean(commonDir)
	if !isBareCommonDir(commonDir) {
		return nil, false
	}

	worktrees := []Worktree{{Path: commonDir, IsBare: true}}
	entries, err := os.ReadDir(filepath.Join(commonDir, "worktrees"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return worktrees, true
		}
		return nil, false
	}

	resolver := refResolver{commonDir: commonDir}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		wt, ok := readLinkedWorktree(filepath.Join(commonDir, "worktrees", entry.Name()), &resolver)
		if !ok {
			return nil, false
		}
		worktrees = append(worktrees, wt)
	}
	return worktrees, true
}

func isBareCommonDir(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, "HEAD")); err != nil || info.IsDir() {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.EqualFold(line, "bare = true") {
			return true
		}
	}
	return false
}

type refResolver struct {
	commonDir     string
	packedRefs    map[string]string
	packedRefsOK  bool
	packedRefsSet bool
}

func readLinkedWorktree(adminDir string, refs *refResolver) (Worktree, bool) {
	gitDirData, err := os.ReadFile(filepath.Join(adminDir, "gitdir"))
	if err != nil {
		return Worktree{}, false
	}
	gitDir := strings.TrimSpace(string(gitDirData))
	if gitDir == "" {
		return Worktree{}, false
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(adminDir, gitDir)
	}

	wtPath := filepath.Clean(gitDir)
	if filepath.Base(wtPath) == ".git" {
		wtPath = filepath.Dir(wtPath)
	}
	wt := Worktree{Path: wtPath}

	headData, err := os.ReadFile(filepath.Join(adminDir, "HEAD"))
	if err != nil {
		return Worktree{}, false
	}
	head := strings.TrimSpace(string(headData))
	if head == "" {
		return Worktree{}, false
	}
	if ref, ok := strings.CutPrefix(head, "ref:"); ok {
		ref = strings.TrimSpace(ref)
		branch, ok := strings.CutPrefix(ref, "refs/heads/")
		if !ok || branch == "" {
			return Worktree{}, false
		}
		resolved, ok := refs.resolve(ref)
		if !ok {
			return Worktree{}, false
		}
		wt.Branch = branch
		wt.Head = resolved
		return wt, true
	}

	wt.Branch = DetachedBranch
	wt.Head = head
	wt.Detached = true
	return wt, true
}

func (r *refResolver) resolve(ref string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(r.commonDir, filepath.FromSlash(ref)))
	if err == nil {
		value := strings.TrimSpace(string(data))
		return value, value != ""
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", false
	}

	refs, ok := r.loadPackedRefs()
	if !ok {
		return "", false
	}
	value := refs[ref]
	return value, value != ""
}

func (r *refResolver) loadPackedRefs() (map[string]string, bool) {
	if r.packedRefsSet {
		return r.packedRefs, r.packedRefsOK
	}

	r.packedRefsSet = true
	data, err := os.ReadFile(filepath.Join(r.commonDir, "packed-refs"))
	if err != nil {
		r.packedRefs = map[string]string{}
		r.packedRefsOK = errors.Is(err, os.ErrNotExist)
		return r.packedRefs, r.packedRefsOK
	}

	r.packedRefs = make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		sha, name, ok := strings.Cut(line, " ")
		if ok && sha != "" && name != "" {
			r.packedRefs[name] = sha
		}
	}
	r.packedRefsOK = true
	return r.packedRefs, true
}

func parsePorcelain(output string) []Worktree {
	var worktrees []Worktree
	for _, block := range strings.Split(strings.TrimSpace(output), "\n\n") {
		var wt Worktree
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "worktree "):
				wt.Path = strings.TrimPrefix(line, "worktree ")
			case strings.HasPrefix(line, "HEAD "):
				wt.Head = strings.TrimPrefix(line, "HEAD ")
			case strings.HasPrefix(line, "branch "):
				wt.Branch = strings.TrimPrefix(line, "branch refs/heads/")
			case line == "bare":
				wt.IsBare = true
			case line == "detached":
				wt.Branch = DetachedBranch
				wt.Detached = true
			}
		}
		if wt.Path != "" {
			worktrees = append(worktrees, wt)
		}
	}
	return worktrees
}
