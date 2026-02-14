package cli

import "testing"

func TestRepoNameFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		// HTTPS
		{"https://github.com/org/repo.git", "repo"},
		{"https://github.com/org/repo", "repo"},
		{"https://github.com/org/my-repo.git", "my-repo"},

		// SSH
		{"git@github.com:org/repo.git", "repo"},
		{"git@github.com:org/repo", "repo"},
		{"git@github.com:org/my-repo.git", "my-repo"},

		// Nested paths
		{"git@github.com:org/sub/repo.git", "repo"},
		{"https://github.com/org/sub/repo.git", "repo"},

		// Edge cases
		{"repo.git", "repo"},
		{"repo", "repo"},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			got := repoNameFromURL(tt.url)
			if got != tt.want {
				t.Errorf("repoNameFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
