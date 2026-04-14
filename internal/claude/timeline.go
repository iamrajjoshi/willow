package claude

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type TimelineEntry struct {
	Status Status    `json:"s"`
	Time   time.Time `json:"t"`
}

// TimelinePath returns the path to the timeline JSONL file for a session.
func TimelinePath(repoName, worktreeDir, sessionID string) string {
	return filepath.Join(StatusDir(), repoName, worktreeDir, sessionID+".timeline")
}

// ReadTimeline reads timeline entries within a time window.
func ReadTimeline(repoName, worktreeDir, sessionID string, since time.Time) ([]TimelineEntry, error) {
	path := TimelinePath(repoName, worktreeDir, sessionID)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []TimelineEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry TimelineEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if !entry.Time.Before(since) {
			entries = append(entries, entry)
		}
	}
	return entries, scanner.Err()
}

// Sparkline renders a compact sparkline string from timeline entries.
// buckets is the number of character positions, window is the total time span.
// Returns a string of ANSI-colored block elements. The visual width is always
// equal to buckets, but the byte length is larger due to escape codes.
func Sparkline(entries []TimelineEntry, buckets int, window time.Duration) string {
	if len(entries) == 0 {
		return strings.Repeat("\033[2m\u00b7\033[0m", buckets)
	}

	now := time.Now()
	start := now.Add(-window)
	bucketDur := window / time.Duration(buckets)

	var b strings.Builder
	for i := 0; i < buckets; i++ {
		bucketStart := start.Add(time.Duration(i) * bucketDur)
		bucketEnd := bucketStart.Add(bucketDur)

		dominant := dominantStatus(entries, bucketStart, bucketEnd)
		switch dominant {
		case StatusBusy:
			b.WriteString("\033[32m\u2588\033[0m")
		case StatusWait:
			b.WriteString("\033[33m\u2593\033[0m")
		case StatusDone:
			b.WriteString("\033[36m\u2591\033[0m")
		default:
			b.WriteString("\033[2m\u00b7\033[0m")
		}
	}
	return b.String()
}

// dominantStatus finds the status that occupies the most time in [start, end).
func dominantStatus(entries []TimelineEntry, start, end time.Time) Status {
	// Find the most recent entry at or before start as the initial status
	var currentStatus Status
	for _, e := range entries {
		if !e.Time.After(start) {
			currentStatus = e.Status
		}
	}

	durations := make(map[Status]time.Duration)
	cursor := start

	for _, e := range entries {
		if e.Time.Before(start) || !e.Time.Before(end) {
			continue
		}
		if currentStatus != "" {
			durations[currentStatus] += e.Time.Sub(cursor)
		}
		currentStatus = e.Status
		cursor = e.Time
	}
	// Remainder of bucket
	if currentStatus != "" {
		durations[currentStatus] += end.Sub(cursor)
	}

	var maxStatus Status
	var maxDur time.Duration
	for s, d := range durations {
		if d > maxDur {
			maxDur = d
			maxStatus = s
		}
	}
	return maxStatus
}
