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
