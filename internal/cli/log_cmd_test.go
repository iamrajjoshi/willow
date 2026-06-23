package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	activitylog "github.com/iamrajjoshi/willow/internal/log"
	"github.com/iamrajjoshi/willow/internal/termfmt"
	"github.com/iamrajjoshi/willow/internal/ui"
)

func TestParseDurationSupportsDaysAndReportsBadInput(t *testing.T) {
	got, err := parseDuration("7d")
	if err != nil {
		t.Fatalf("parseDuration(7d) error = %v", err)
	}
	if got != 7*24*time.Hour {
		t.Fatalf("parseDuration(7d) = %s, want 168h", got)
	}

	got, err = parseDuration("30m")
	if err != nil {
		t.Fatalf("parseDuration(30m) error = %v", err)
	}
	if got != 30*time.Minute {
		t.Fatalf("parseDuration(30m) = %s, want 30m", got)
	}

	if _, err := parseDuration("soon"); err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("parseDuration(soon) error = %v, want invalid duration", err)
	}
}

func TestFormatMetadataSkipsEmptyAndTruncatesLongValues(t *testing.T) {
	got := formatMetadata(map[string]string{
		"empty": "",
		"short": "ok",
		"long":  strings.Repeat("x", 70),
	})
	if strings.Contains(got, "empty=") {
		t.Fatalf("formatMetadata should skip empty values, got %q", got)
	}
	if !strings.Contains(got, "short=ok") {
		t.Fatalf("formatMetadata missing short value: %q", got)
	}
	if !strings.Contains(got, "long="+strings.Repeat("x", 57)+"...") {
		t.Fatalf("formatMetadata missing truncated long value: %q", got)
	}
}

func TestFormatLogLinesFitsNarrowWidth(t *testing.T) {
	u := &ui.UI{}
	ts := time.Date(2026, 6, 22, 12, 30, 0, 0, time.UTC)
	events := []activitylog.Event{
		{
			Action:    "create",
			Repo:      "evergreen",
			Branch:    "raj--tprm-464--backend-validate-review-risk-subtype",
			Timestamp: ts,
			Metadata:  map[string]string{"path": strings.Repeat("x", 80)},
		},
	}
	lines := formatLogLines(u, events, 72)
	for _, line := range lines {
		if got := termfmt.VisibleWidth(line); got > 72 {
			t.Fatalf("line width = %d, want <= 72:\n%s", got, termfmt.StripANSI(line))
		}
	}
	plain := termfmt.StripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{ts.Local().Format("Jan 02 15:04"), "create", "evergreen"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("log line missing %q:\n%s", want, plain)
		}
	}
}

func TestLogCommandJSONAppliesRepoBranchSinceAndLimitFilters(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Now().UTC()
	events := []activitylog.Event{
		{Action: "create", Repo: "repo", Branch: "feature", Timestamp: now.Add(-10 * time.Minute), Metadata: map[string]string{"base": "main"}},
		{Action: "remove", Repo: "repo", Branch: "other", Timestamp: now.Add(-9 * time.Minute)},
		{Action: "create", Repo: "other", Branch: "feature", Timestamp: now.Add(-8 * time.Minute)},
		{Action: "old", Repo: "repo", Branch: "feature", Timestamp: now.Add(-48 * time.Hour)},
	}
	for _, e := range events {
		if err := activitylog.Append(e); err != nil {
			t.Fatalf("append log event: %v", err)
		}
	}

	out, err := captureStdout(t, func() error {
		return runApp("log", "--repo", "repo", "--branch", "feature", "--since", "24h", "--limit", "5", "--json")
	})
	if err != nil {
		t.Fatalf("log command failed: %v", err)
	}

	var got []activitylog.Event
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("log JSON output failed to parse: %v\n%s", err, out)
	}
	if len(got) != 1 {
		t.Fatalf("log command returned %d events, want 1: %+v", len(got), got)
	}
	if got[0].Action != "create" || got[0].Repo != "repo" || got[0].Branch != "feature" {
		t.Fatalf("unexpected filtered log event: %+v", got[0])
	}
}

func TestLogCommandHumanOutputIncludesMetadata(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := activitylog.Append(activitylog.Event{
		Action:   "sync",
		Repo:     "repo",
		Branch:   "feature",
		Metadata: map[string]string{"parent": "main"},
	}); err != nil {
		t.Fatalf("append log event: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runApp("log", "--repo", "repo")
	})
	if err != nil {
		t.Fatalf("log command failed: %v", err)
	}
	for _, want := range []string{"sync", "repo", "feature", "parent=main"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log output missing %q:\n%s", want, out)
		}
	}
}
