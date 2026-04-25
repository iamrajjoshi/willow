package stack

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSave(t *testing.T) {
	dir := t.TempDir()

	s := &Stack{Parents: map[string]string{
		"feature-a": "main",
		"feature-b": "feature-a",
	}}

	if err := s.Save(dir); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded := Load(dir)
	if loaded.Parent("feature-a") != "main" {
		t.Errorf("expected feature-a parent=main, got %q", loaded.Parent("feature-a"))
	}
	if loaded.Parent("feature-b") != "feature-a" {
		t.Errorf("expected feature-b parent=feature-a, got %q", loaded.Parent("feature-b"))
	}
}

func TestSaveEmptyRemovesFile(t *testing.T) {
	dir := t.TempDir()

	s := &Stack{Parents: map[string]string{"a": "main"}}
	s.Save(dir)

	s.Remove("a")
	s.Save(dir)

	if _, err := os.Stat(filepath.Join(dir, "branches.json")); !os.IsNotExist(err) {
		t.Error("branches.json should be removed when stack is empty")
	}
}

func TestChildren(t *testing.T) {
	s := &Stack{Parents: map[string]string{
		"a": "main",
		"b": "a",
		"c": "a",
		"d": "b",
	}}

	children := s.Children("a")
	if len(children) != 2 || children[0] != "b" || children[1] != "c" {
		t.Errorf("Children(a) = %v, want [b c]", children)
	}

	children = s.Children("b")
	if len(children) != 1 || children[0] != "d" {
		t.Errorf("Children(b) = %v, want [d]", children)
	}
}

func TestDescendants(t *testing.T) {
	s := &Stack{Parents: map[string]string{
		"a": "main",
		"b": "a",
		"c": "b",
	}}

	desc := s.Descendants("a")
	if len(desc) != 2 {
		t.Errorf("Descendants(a) = %v, want [b c]", desc)
	}
}

func TestRoots(t *testing.T) {
	s := &Stack{Parents: map[string]string{
		"a": "main",
		"b": "a",
		"x": "develop",
	}}

	roots := s.Roots()
	if len(roots) != 2 || roots[0] != "a" || roots[1] != "x" {
		t.Errorf("Roots() = %v, want [a x]", roots)
	}
}

func TestTopoSort(t *testing.T) {
	s := &Stack{Parents: map[string]string{
		"a": "main",
		"b": "a",
		"c": "b",
	}}

	sorted := s.TopoSort()
	if len(sorted) != 3 {
		t.Fatalf("TopoSort() = %v, want 3 elements", sorted)
	}
	// a must come before b, b before c
	idx := make(map[string]int)
	for i, b := range sorted {
		idx[b] = i
	}
	if idx["a"] >= idx["b"] || idx["b"] >= idx["c"] {
		t.Errorf("TopoSort() = %v, wrong order", sorted)
	}
}

func TestSubtreeSort(t *testing.T) {
	s := &Stack{Parents: map[string]string{
		"a": "main",
		"b": "a",
		"c": "b",
		"x": "main",
	}}

	subtree := s.SubtreeSort("a")
	if len(subtree) != 3 {
		t.Fatalf("SubtreeSort(a) = %v, want [a b c]", subtree)
	}
	if subtree[0] != "a" {
		t.Errorf("first element should be a, got %s", subtree[0])
	}
}

func TestUpdate(t *testing.T) {
	dir := t.TempDir()

	// Seed initial state
	s := &Stack{Parents: map[string]string{"a": "main"}}
	if err := s.Save(dir); err != nil {
		t.Fatalf("seed save: %v", err)
	}

	// Use Update to add a new branch under the lock
	if err := Update(dir, func(s *Stack) {
		s.SetParent("b", "a")
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	loaded := Load(dir)
	if loaded.Parent("b") != "a" {
		t.Errorf("expected b parent=a, got %q", loaded.Parent("b"))
	}
	if loaded.Parent("a") != "main" {
		t.Errorf("expected a parent=main, got %q", loaded.Parent("a"))
	}
}

func TestRemoveReparentsChildren(t *testing.T) {
	s := &Stack{Parents: map[string]string{
		"a": "main",
		"b": "a",
		"c": "a",
	}}

	s.Remove("a")

	if s.Parent("b") != "main" {
		t.Errorf("b should be re-parented to main, got %q", s.Parent("b"))
	}
	if s.Parent("c") != "main" {
		t.Errorf("c should be re-parented to main, got %q", s.Parent("c"))
	}
	if s.IsTracked("a") {
		t.Error("a should be removed")
	}
}

func TestRenameUpdatesBranchAndChildren(t *testing.T) {
	s := &Stack{Parents: map[string]string{
		"a": "main",
		"b": "a",
		"c": "b",
	}}

	s.Rename("a", "renamed")

	if s.IsTracked("a") {
		t.Error("old branch should not remain tracked")
	}
	if s.Parent("renamed") != "main" {
		t.Errorf("renamed parent = %q, want main", s.Parent("renamed"))
	}
	if s.Parent("b") != "renamed" {
		t.Errorf("b parent = %q, want renamed", s.Parent("b"))
	}
	if s.Parent("c") != "b" {
		t.Errorf("c parent = %q, want b", s.Parent("c"))
	}
}

func TestIsEmpty(t *testing.T) {
	if !(&Stack{Parents: map[string]string{}}).IsEmpty() {
		t.Fatal("empty stack should report IsEmpty")
	}
	if (&Stack{Parents: map[string]string{"a": "main"}}).IsEmpty() {
		t.Fatal("non-empty stack should not report IsEmpty")
	}
}

func TestTreeLinesFiltersToVisibleBranches(t *testing.T) {
	s := &Stack{Parents: map[string]string{
		"a": "main",
		"b": "a",
		"c": "b",
		"x": "main",
	}}

	lines := s.TreeLines(map[string]bool{"b": true, "c": true})
	if len(lines) != 2 {
		t.Fatalf("TreeLines() returned %d lines, want 2: %+v", len(lines), lines)
	}
	if lines[0].Branch != "b" || lines[0].Prefix != "└─ " || lines[0].Depth != 1 {
		t.Fatalf("first tree line = %+v, want b with skipped-root prefix", lines[0])
	}
	if lines[1].Branch != "c" || lines[1].Depth != 2 {
		t.Fatalf("second tree line = %+v, want c descendant", lines[1])
	}
}

func TestTreeLinesEmptyStack(t *testing.T) {
	s := &Stack{Parents: map[string]string{}}
	if got := s.TreeLines(map[string]bool{"a": true}); got != nil {
		t.Fatalf("empty TreeLines() = %+v, want nil", got)
	}
}
