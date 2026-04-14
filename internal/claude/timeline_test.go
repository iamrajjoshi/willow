package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSparkline_AllBusy(t *testing.T) {
	now := time.Now()
	entries := []TimelineEntry{
		{Status: StatusBusy, Time: now.Add(-60 * time.Minute)},
	}
	result := Sparkline(entries, 10, 60*time.Minute)
	// Each bucket should be a green full block + ANSI codes
	// Visual width = 10, all full blocks
	plain := stripANSI(result)
	if plain != strings.Repeat("\u2588", 10) {
		t.Errorf("all BUSY sparkline = %q, want 10 full blocks", plain)
	}
}

func TestSparkline_BusyThenWait(t *testing.T) {
	now := time.Now()
	entries := []TimelineEntry{
		{Status: StatusBusy, Time: now.Add(-60 * time.Minute)},
		{Status: StatusWait, Time: now.Add(-30 * time.Minute)},
	}
	result := Sparkline(entries, 10, 60*time.Minute)
	plain := stripANSI(result)
	// First 5 buckets should be full block (BUSY), last 5 should be dark shade (WAIT)
	if len([]rune(plain)) != 10 {
		t.Fatalf("sparkline has %d runes, want 10", len([]rune(plain)))
	}
	for i, r := range []rune(plain) {
		if i < 5 {
			if r != '\u2588' {
				t.Errorf("bucket %d = %q, want full block (BUSY)", i, string(r))
			}
		} else {
			if r != '\u2593' {
				t.Errorf("bucket %d = %q, want dark shade (WAIT)", i, string(r))
			}
		}
	}
}

func TestSparkline_Empty(t *testing.T) {
	result := Sparkline(nil, 10, 60*time.Minute)
	plain := stripANSI(result)
	if plain != strings.Repeat("\u00b7", 10) {
		t.Errorf("empty sparkline = %q, want 10 middle dots", plain)
	}
}

func TestDominantStatus_FullBucket(t *testing.T) {
	now := time.Now()
	entries := []TimelineEntry{
		{Status: StatusBusy, Time: now.Add(-10 * time.Minute)},
	}
	start := now.Add(-5 * time.Minute)
	end := now

	got := dominantStatus(entries, start, end)
	if got != StatusBusy {
		t.Errorf("dominantStatus = %q, want BUSY", got)
	}
}

func TestDominantStatus_SplitBucket(t *testing.T) {
	now := time.Now()
	entries := []TimelineEntry{
		{Status: StatusBusy, Time: now.Add(-10 * time.Minute)},
		{Status: StatusWait, Time: now.Add(-3 * time.Minute)},
	}
	start := now.Add(-10 * time.Minute)
	end := now

	got := dominantStatus(entries, start, end)
	// BUSY occupies 7 min, WAIT occupies 3 min
	if got != StatusBusy {
		t.Errorf("dominantStatus = %q, want BUSY (7min vs 3min)", got)
	}
}

func TestDominantStatus_EmptyBucket(t *testing.T) {
	now := time.Now()
	start := now.Add(-5 * time.Minute)
	end := now

	got := dominantStatus(nil, start, end)
	if got != "" {
		t.Errorf("dominantStatus = %q, want empty", got)
	}
}

func TestReadTimeline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	repoName := "myrepo"
	wtDir := "feat-auth"
	sessionID := "sess-1"

	dir := filepath.Join(home, ".willow", "status", repoName, wtDir)
	os.MkdirAll(dir, 0o755)

	now := time.Now().UTC()
	entries := []TimelineEntry{
		{Status: StatusBusy, Time: now.Add(-30 * time.Minute)},
		{Status: StatusWait, Time: now.Add(-15 * time.Minute)},
		{Status: StatusDone, Time: now.Add(-5 * time.Minute)},
	}

	path := filepath.Join(dir, sessionID+".timeline")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, _ := json.Marshal(e)
		f.Write(data)
		f.Write([]byte("\n"))
	}
	f.Close()

	// Read all entries (since 1 hour ago)
	got, err := ReadTimeline(repoName, wtDir, sessionID, now.Add(-60*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("ReadTimeline returned %d entries, want 3", len(got))
	}

	// Read only recent entries (since 20 min ago)
	got, err = ReadTimeline(repoName, wtDir, sessionID, now.Add(-20*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("ReadTimeline (since 20m) returned %d entries, want 2", len(got))
	}
}

func TestReadTimeline_MissingFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := ReadTimeline("norepo", "nowt", "nosess", time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil entries for missing file, got %v", got)
	}
}

func TestTimelinePath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	path := TimelinePath("myrepo", "feat-auth", "sess-1")
	want := filepath.Join(home, ".willow", "status", "myrepo", "feat-auth", "sess-1.timeline")
	if path != want {
		t.Errorf("TimelinePath = %q, want %q", path, want)
	}
}

// stripANSI removes ANSI escape sequences for plain-text comparison.
func stripANSI(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
