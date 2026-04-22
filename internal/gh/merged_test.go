package gh

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestMergedWorktreeSet_MergeEligibility(t *testing.T) {
	now := time.Date(2026, 4, 21, 18, 30, 0, 0, time.UTC)

	tests := []struct {
		name       string
		base       string
		branch     string
		head       string
		pr         *PRInfo
		wantMerged bool
	}{
		{
			name:   "merged pr with matching head sha",
			base:   "master",
			branch: "feature",
			head:   "abc123",
			pr: &PRInfo{
				Number:      101,
				Branch:      "feature",
				HeadRefOID:  "abc123",
				BaseRefName: "master",
				State:       "MERGED",
				MergedAt:    "2026-04-21T18:08:47Z",
			},
			wantMerged: true,
		},
		{
			name:   "open pr is not merged",
			base:   "master",
			branch: "feature",
			head:   "abc123",
			pr: &PRInfo{
				Number:      101,
				Branch:      "feature",
				HeadRefOID:  "abc123",
				BaseRefName: "master",
				State:       "OPEN",
			},
		},
		{
			name:   "merged pr against different base is ignored",
			base:   "master",
			branch: "feature",
			head:   "abc123",
			pr: &PRInfo{
				Number:      101,
				Branch:      "feature",
				HeadRefOID:  "abc123",
				BaseRefName: "develop",
				State:       "MERGED",
				MergedAt:    "2026-04-21T18:08:47Z",
			},
		},
		{
			name:   "merged pr with mismatched head sha is ignored",
			base:   "master",
			branch: "feature",
			head:   "abc123",
			pr: &PRInfo{
				Number:      101,
				Branch:      "feature",
				HeadRefOID:  "def456",
				BaseRefName: "master",
				State:       "MERGED",
				MergedAt:    "2026-04-21T18:08:47Z",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prepareMergedWorktreeTestEnv(t, now, func(_ string, branches []string) ([]*PRInfo, error) {
				if len(branches) != 1 || branches[0] != tt.branch {
					t.Fatalf("search branches = %v, want [%s]", branches, tt.branch)
				}
				if tt.pr == nil {
					return nil, nil
				}
				return []*PRInfo{tt.pr}, nil
			})

			got := MergedWorktreeSet("/fake/repo", tt.base, map[string]string{
				tt.branch: tt.head,
			})
			if got[tt.branch] != tt.wantMerged {
				t.Fatalf("MergedWorktreeSet()[%q] = %v, want %v", tt.branch, got[tt.branch], tt.wantMerged)
			}
		})
	}
}

func TestMergedWorktreeSet_NegativeResultsAreCached(t *testing.T) {
	now := time.Date(2026, 4, 21, 18, 30, 0, 0, time.UTC)
	calls := 0

	prepareMergedWorktreeTestEnv(t, now, func(_ string, branches []string) ([]*PRInfo, error) {
		calls++
		if len(branches) != 1 || branches[0] != "feature" {
			t.Fatalf("search branches = %v, want [feature]", branches)
		}
		return nil, nil
	})

	heads := map[string]string{"feature": "abc123"}
	first := MergedWorktreeSet("/fake/repo", "master", heads)
	second := MergedWorktreeSet("/fake/repo", "master", heads)

	if len(first) != 0 || len(second) != 0 {
		t.Fatalf("expected no merged branches, got first=%v second=%v", first, second)
	}
	if calls != 1 {
		t.Fatalf("search calls = %d, want 1", calls)
	}
}

func TestMergedWorktreeSet_FreshCacheIsReused(t *testing.T) {
	now := time.Date(2026, 4, 21, 18, 30, 0, 0, time.UTC)
	calls := 0

	prepareMergedWorktreeTestEnv(t, now, func(_ string, branches []string) ([]*PRInfo, error) {
		calls++
		return []*PRInfo{{
			Number:      101,
			Branch:      "feature",
			HeadRefOID:  "abc123",
			BaseRefName: "master",
			State:       "MERGED",
			MergedAt:    "2026-04-21T18:08:47Z",
		}}, nil
	})

	heads := map[string]string{"feature": "abc123"}
	for i := 0; i < 2; i++ {
		got := MergedWorktreeSet("/fake/repo", "master", heads)
		if !got["feature"] {
			t.Fatalf("call %d: expected feature to be merged, got %v", i+1, got)
		}
	}

	if calls != 1 {
		t.Fatalf("search calls = %d, want 1", calls)
	}
}

func TestMergedWorktreeSet_StaleCacheRefreshes(t *testing.T) {
	start := time.Date(2026, 4, 21, 18, 30, 0, 0, time.UTC)
	current := start
	calls := 0

	prepareMergedWorktreeTestEnv(t, current, func(_ string, branches []string) ([]*PRInfo, error) {
		calls++
		if calls == 1 {
			return nil, nil
		}
		return []*PRInfo{{
			Number:      102,
			Branch:      "feature",
			HeadRefOID:  "abc123",
			BaseRefName: "master",
			State:       "MERGED",
			MergedAt:    "2026-04-21T18:45:00Z",
		}}, nil
	})

	mergedWorktreeNow = func() time.Time { return current }
	heads := map[string]string{"feature": "abc123"}

	if got := MergedWorktreeSet("/fake/repo", "master", heads); got["feature"] {
		t.Fatalf("first call unexpectedly marked feature merged: %v", got)
	}

	current = start.Add(mergedWorktreeCacheTTL + time.Second)
	got := MergedWorktreeSet("/fake/repo", "master", heads)
	if !got["feature"] {
		t.Fatalf("expected stale cache refresh to mark feature merged, got %v", got)
	}
	if calls != 2 {
		t.Fatalf("search calls = %d, want 2", calls)
	}
}

func TestMergedWorktreeSet_MissingBranchEntriesRefreshIndividually(t *testing.T) {
	now := time.Date(2026, 4, 21, 18, 30, 0, 0, time.UTC)
	calls := 0

	prepareMergedWorktreeTestEnv(t, now, func(_ string, branches []string) ([]*PRInfo, error) {
		calls++
		slices.Sort(branches)
		switch calls {
		case 1:
			if !slices.Equal(branches, []string{"feature-a"}) {
				t.Fatalf("first search branches = %v, want [feature-a]", branches)
			}
			return []*PRInfo{{
				Number:      101,
				Branch:      "feature-a",
				HeadRefOID:  "sha-a",
				BaseRefName: "master",
				State:       "MERGED",
				MergedAt:    "2026-04-21T18:08:47Z",
			}}, nil
		case 2:
			if !slices.Equal(branches, []string{"feature-b"}) {
				t.Fatalf("second search branches = %v, want [feature-b]", branches)
			}
			return []*PRInfo{{
				Number:      102,
				Branch:      "feature-b",
				HeadRefOID:  "sha-b",
				BaseRefName: "master",
				State:       "MERGED",
				MergedAt:    "2026-04-21T18:09:47Z",
			}}, nil
		default:
			t.Fatalf("unexpected search call %d for branches %v", calls, branches)
			return nil, nil
		}
	})

	first := MergedWorktreeSet("/fake/repo", "master", map[string]string{
		"feature-a": "sha-a",
	})
	if !first["feature-a"] {
		t.Fatalf("expected feature-a to be merged, got %v", first)
	}

	second := MergedWorktreeSet("/fake/repo", "master", map[string]string{
		"feature-a": "sha-a",
		"feature-b": "sha-b",
	})
	if !second["feature-a"] || !second["feature-b"] {
		t.Fatalf("expected both branches merged after refresh, got %v", second)
	}
	if calls != 2 {
		t.Fatalf("search calls = %d, want 2", calls)
	}
}

func prepareMergedWorktreeTestEnv(t *testing.T, now time.Time, search func(dir string, branches []string) ([]*PRInfo, error)) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	origNow := mergedWorktreeNow
	origCLI := mergedWorktreeCLI
	origSearch := mergedWorktreeSearchPRs
	origKey := mergedWorktreeRepoKey

	mergedWorktreeNow = func() time.Time { return now }
	mergedWorktreeCLI = func() bool { return true }
	mergedWorktreeSearchPRs = search
	mergedWorktreeRepoKey = func(string) string { return "test-repo" }

	t.Cleanup(func() {
		mergedWorktreeNow = origNow
		mergedWorktreeCLI = origCLI
		mergedWorktreeSearchPRs = origSearch
		mergedWorktreeRepoKey = origKey
	})

	if err := os.MkdirAll(filepath.Join(home, ".willow", "cache"), 0o755); err != nil {
		t.Fatalf("mkdir cache dir: %v", err)
	}
}
