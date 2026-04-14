package stack

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
)

// Stack tracks parent-child relationships between branches for stacked PRs.
// Stored as branches.json in the bare repo directory.
type Stack struct {
	Parents map[string]string `json:"parents"` // branch → parent
}

// TreeLine represents a branch with its tree-drawing prefix for display.
type TreeLine struct {
	Branch string
	Prefix string // e.g., "├─ ", "│  └─ ", ""
	Depth  int
}

func filePath(bareDir string) string {
	return filepath.Join(bareDir, "branches.json")
}

// Load reads the stack from branches.json. Returns an empty stack if the file doesn't exist.
func Load(bareDir string) *Stack {
	s := &Stack{Parents: make(map[string]string)}
	data, err := os.ReadFile(filePath(bareDir))
	if err != nil {
		return s
	}
	// Support both {"parents": {...}} and flat {...} formats
	var wrapped struct {
		Parents map[string]string `json:"parents"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Parents != nil {
		s.Parents = wrapped.Parents
		return s
	}
	// Try flat format
	var flat map[string]string
	if err := json.Unmarshal(data, &flat); err == nil {
		s.Parents = flat
	}
	return s
}

// Save writes the stack to branches.json. Removes the file if the stack is empty.
func (s *Stack) Save(bareDir string) error {
	fp := filePath(bareDir)
	if len(s.Parents) == 0 {
		err := os.Remove(fp)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	data, err := json.MarshalIndent(s.Parents, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(fp, append(data, '\n'))
}

// atomicWrite writes data to a temp file then renames it to path,
// preventing corruption from interrupted writes.
func atomicWrite(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Update performs a locked read-modify-write cycle on branches.json.
// The lock prevents concurrent willow commands from corrupting the file.
func Update(bareDir string, fn func(*Stack)) error {
	lockPath := filePath(bareDir) + ".lock"
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open lock file: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("lock branches.json: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	s := Load(bareDir)
	fn(s)
	return s.Save(bareDir)
}

func (s *Stack) SetParent(branch, parent string) {
	s.Parents[branch] = parent
}

// Remove deletes a branch from the stack and re-parents its children to
// the removed branch's parent.
func (s *Stack) Remove(branch string) {
	parent := s.Parents[branch]
	for child, p := range s.Parents {
		if p == branch {
			if parent != "" {
				s.Parents[child] = parent
			} else {
				delete(s.Parents, child)
			}
		}
	}
	delete(s.Parents, branch)
}

func (s *Stack) Parent(branch string) string {
	return s.Parents[branch]
}

func (s *Stack) IsTracked(branch string) bool {
	_, ok := s.Parents[branch]
	return ok
}

// Children returns direct children of a branch (branches whose parent is this branch).
func (s *Stack) Children(branch string) []string {
	var children []string
	for child, parent := range s.Parents {
		if parent == branch {
			children = append(children, child)
		}
	}
	sort.Strings(children)
	return children
}

// Descendants returns all transitive children of a branch.
func (s *Stack) Descendants(branch string) []string {
	var result []string
	queue := s.Children(branch)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		result = append(result, cur)
		queue = append(queue, s.Children(cur)...)
	}
	return result
}

// Roots returns branches whose parent is NOT itself a tracked branch.
// These are the entry points of stacks (e.g., branches based on "main").
func (s *Stack) Roots() []string {
	var roots []string
	for branch, parent := range s.Parents {
		if _, parentTracked := s.Parents[parent]; !parentTracked {
			roots = append(roots, branch)
		}
	}
	sort.Strings(roots)
	return roots
}

// TopoSort returns all tracked branches in topological order (parents before children).
func (s *Stack) TopoSort() []string {
	var result []string
	visited := make(map[string]bool)

	var visit func(branch string)
	visit = func(branch string) {
		if visited[branch] {
			return
		}
		visited[branch] = true
		// Visit parent first (if it's tracked)
		if parent, ok := s.Parents[branch]; ok {
			if _, parentTracked := s.Parents[parent]; parentTracked {
				visit(parent)
			}
		}
		result = append(result, branch)
	}

	// Process roots first for stable ordering
	for _, root := range s.Roots() {
		visit(root)
	}
	// Then any remaining
	for branch := range s.Parents {
		visit(branch)
	}

	return result
}

// SubtreeSort returns a branch and its descendants in topological order.
func (s *Stack) SubtreeSort(root string) []string {
	result := []string{root}
	result = append(result, s.topoChildren(root)...)
	return result
}

func (s *Stack) topoChildren(branch string) []string {
	var result []string
	for _, child := range s.Children(branch) {
		result = append(result, child)
		result = append(result, s.topoChildren(child)...)
	}
	return result
}

// IsEmpty returns true if no branches are tracked.
func (s *Stack) IsEmpty() bool {
	return len(s.Parents) == 0
}

// TreeLines generates display lines with tree-drawing characters for the given
// set of branches. Only branches present in the provided set are included.
func (s *Stack) TreeLines(branchSet map[string]bool) []TreeLine {
	if s.IsEmpty() {
		return nil
	}

	var lines []TreeLine
	// Process each root and its subtree
	for _, root := range s.Roots() {
		if !branchSet[root] {
			// Still process children that might have worktrees
			s.collectTreeLines(&lines, root, "", 0, branchSet, true)
			continue
		}
		s.collectTreeLines(&lines, root, "", 0, branchSet, false)
	}
	return lines
}

func (s *Stack) collectTreeLines(lines *[]TreeLine, branch, prefix string, depth int, branchSet map[string]bool, skipSelf bool) {
	if !skipSelf && branchSet[branch] {
		*lines = append(*lines, TreeLine{
			Branch: branch,
			Prefix: prefix,
			Depth:  depth,
		})
	}

	children := s.Children(branch)
	// Filter to children that have worktrees (or have descendants with worktrees)
	var visibleChildren []string
	for _, child := range children {
		if branchSet[child] || s.hasDescendantIn(child, branchSet) {
			visibleChildren = append(visibleChildren, child)
		}
	}

	for i, child := range visibleChildren {
		isLast := i == len(visibleChildren)-1
		var childPrefix, nextPrefix string
		if depth == 0 && skipSelf {
			// Root is not shown, children are at top level
			if isLast {
				childPrefix = "└─ "
				nextPrefix = "   "
			} else {
				childPrefix = "├─ "
				nextPrefix = "│  "
			}
		} else if skipSelf {
			childPrefix = prefix
			nextPrefix = prefix
		} else {
			if isLast {
				childPrefix = prefix + "└─ "
				nextPrefix = prefix + "   "
			} else {
				childPrefix = prefix + "├─ "
				nextPrefix = prefix + "│  "
			}
		}
		if branchSet[child] {
			*lines = append(*lines, TreeLine{
				Branch: child,
				Prefix: childPrefix,
				Depth:  depth + 1,
			})
		}
		// Recurse into grandchildren
		for _, grandchild := range s.Children(child) {
			s.collectTreeLines(lines, grandchild, nextPrefix, depth+2, branchSet, false)
		}
	}
}

func (s *Stack) hasDescendantIn(branch string, branchSet map[string]bool) bool {
	for _, child := range s.Children(branch) {
		if branchSet[child] || s.hasDescendantIn(child, branchSet) {
			return true
		}
	}
	return false
}
