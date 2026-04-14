package log

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTestDir(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	// LogDir() derives from WillowHome() which uses $HOME.
}

func TestAppend(t *testing.T) {
	setupTestDir(t)

	e := Event{
		Action:    "create",
		Repo:      "myrepo",
		Branch:    "feature-auth",
		Timestamp: time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
		Metadata:  map[string]string{"base": "main"},
	}

	if err := Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}

	path := monthFile(e.Timestamp)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var got Event
	if err := json.Unmarshal([]byte(lines[0]), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Action != "create" || got.Repo != "myrepo" || got.Branch != "feature-auth" {
		t.Errorf("unexpected event: %+v", got)
	}
	if got.Metadata["base"] != "main" {
		t.Errorf("expected metadata base=main, got %v", got.Metadata)
	}
}

func TestAppendMultiple(t *testing.T) {
	setupTestDir(t)

	ts := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	for i := range 3 {
		e := Event{
			Action:    "create",
			Repo:      "repo",
			Branch:    "branch-" + string(rune('a'+i)),
			Timestamp: ts.Add(time.Duration(i) * time.Minute),
		}
		if err := Append(e); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	path := monthFile(ts)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
}

func TestRead(t *testing.T) {
	setupTestDir(t)

	ts := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	events := []Event{
		{Action: "create", Repo: "repo-a", Branch: "branch-1", Timestamp: ts},
		{Action: "remove", Repo: "repo-a", Branch: "branch-1", Timestamp: ts.Add(time.Minute)},
		{Action: "create", Repo: "repo-b", Branch: "branch-2", Timestamp: ts.Add(2 * time.Minute)},
	}
	for _, e := range events {
		if err := Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	// Read all
	got, err := Read(ReadOpts{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	// Most recent first
	if got[0].Action != "create" || got[0].Repo != "repo-b" {
		t.Errorf("expected most recent first, got %+v", got[0])
	}

	// Filter by repo
	got, err = Read(ReadOpts{Repo: "repo-a"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}

	// Filter by branch
	got, err = Read(ReadOpts{Branch: "branch-2"})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
}

func TestReadLimit(t *testing.T) {
	setupTestDir(t)

	ts := time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC)
	for i := range 10 {
		e := Event{
			Action:    "create",
			Repo:      "repo",
			Branch:    "branch",
			Timestamp: ts.Add(time.Duration(i) * time.Minute),
		}
		if err := Append(e); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}

	got, err := Read(ReadOpts{Limit: 3})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	// Should be the 3 most recent
	if !got[0].Timestamp.After(got[1].Timestamp) {
		t.Errorf("expected descending order")
	}
}

func TestReadMonthlyFiles(t *testing.T) {
	setupTestDir(t)

	march := Event{
		Action:    "create",
		Repo:      "repo",
		Branch:    "march-branch",
		Timestamp: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC),
	}
	april := Event{
		Action:    "create",
		Repo:      "repo",
		Branch:    "april-branch",
		Timestamp: time.Date(2026, 4, 14, 10, 0, 0, 0, time.UTC),
	}

	if err := Append(march); err != nil {
		t.Fatalf("Append march: %v", err)
	}
	if err := Append(april); err != nil {
		t.Fatalf("Append april: %v", err)
	}

	// Verify two separate files exist
	files, _ := filepath.Glob(filepath.Join(LogDir(), "*.jsonl"))
	if len(files) != 2 {
		t.Fatalf("expected 2 monthly files, got %d", len(files))
	}

	// Read all — should get both, april first
	got, err := Read(ReadOpts{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Branch != "april-branch" {
		t.Errorf("expected april first, got %s", got[0].Branch)
	}

	// Read with Since — only april
	got, err = Read(ReadOpts{Since: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Branch != "april-branch" {
		t.Errorf("expected april-branch, got %s", got[0].Branch)
	}
}

func TestReadEmpty(t *testing.T) {
	setupTestDir(t)

	got, err := Read(ReadOpts{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 events, got %d", len(got))
	}
}

func TestAppendDefaultTimestamp(t *testing.T) {
	setupTestDir(t)

	e := Event{
		Action: "create",
		Repo:   "repo",
		Branch: "branch",
	}
	if err := Append(e); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := Read(ReadOpts{})
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
	if got[0].Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}
