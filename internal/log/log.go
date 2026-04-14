package log

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/iamrajjoshi/willow/internal/config"
)

type Event struct {
	Action    string            `json:"action"`
	Repo      string            `json:"repo"`
	Branch    string            `json:"branch"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type ReadOpts struct {
	Repo   string
	Branch string
	Since  time.Time
	Limit  int
}

func LogDir() string {
	return filepath.Join(config.WillowHome(), "log")
}

// monthFile returns the path for a given month's log file.
func monthFile(t time.Time) string {
	return filepath.Join(LogDir(), t.Format("2006-01")+".jsonl")
}

// Append writes an event to the current month's JSONL file.
func Append(e Event) error {
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}

	dir := LogDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	data = append(data, '\n')

	f, err := os.OpenFile(monthFile(e.Timestamp), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	_, err = f.Write(data)
	return err
}

// Read returns events matching the given filters, most recent first.
func Read(opts ReadOpts) ([]Event, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	files, err := listLogFiles()
	if err != nil {
		return nil, err
	}

	var result []Event
	for _, path := range files {
		if len(result) >= limit {
			break
		}

		// Skip files that can't contain events after opts.Since.
		// File name is YYYY-MM.jsonl — if the entire month is before Since,
		// we can skip it. We check the month *after* the file's month.
		if !opts.Since.IsZero() {
			monthStr := strings.TrimSuffix(filepath.Base(path), ".jsonl")
			t, err := time.Parse("2006-01", monthStr)
			if err == nil {
				endOfMonth := t.AddDate(0, 1, 0)
				if endOfMonth.Before(opts.Since) {
					continue
				}
			}
		}

		events, err := readFile(path)
		if err != nil {
			continue
		}

		for _, e := range events {
			if !opts.Since.IsZero() && e.Timestamp.Before(opts.Since) {
				continue
			}
			if opts.Repo != "" && e.Repo != opts.Repo {
				continue
			}
			if opts.Branch != "" && e.Branch != opts.Branch {
				continue
			}
			result = append(result, e)
		}
	}

	// Sort by timestamp descending (most recent first).
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// listLogFiles returns all JSONL files in the log directory,
// sorted newest first (by filename which is YYYY-MM).
func listLogFiles() ([]string, error) {
	dir := LogDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}

	// Sort descending by name (YYYY-MM sorts chronologically).
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	return files, nil
}

// readFile reads all events from a single JSONL file.
func readFile(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e Event
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, scanner.Err()
}
