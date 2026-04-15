package cli

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Fix the login validation bug", "fix-the-login-validation-bug"},
		{"Add retry logic to the API client", "add-retry-logic-to-the"},
		{"simple", "simple"},
		{"UPPERCASE WORDS HERE", "uppercase-words-here"},
		{"special! chars@ here#", "special-chars-here"},
		{"", ""},
		{"a b c d e f g h", "a-b-c-d-e"},
		{"a-very-long-word-that-exceeds-the-fifty-character-limit-by-quite-a-bit", "a-very-long-word-that-exceeds-the-fifty-character"},
	}
	for _, tt := range tests {
		got := slugify(tt.input)
		if got != tt.want {
			t.Errorf("slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncatePrompt(t *testing.T) {
	short := "Fix the bug"
	if got := truncatePrompt(short); got != short {
		t.Errorf("truncatePrompt(%q) = %q, want unchanged", short, got)
	}

	long := "This is a very long prompt that exceeds the eighty character limit and should be truncated with an ellipsis at the end"
	got := truncatePrompt(long)
	if len(got) != 80 {
		t.Errorf("truncatePrompt(long) length = %d, want 80", len(got))
	}
	if got[77:] != "..." {
		t.Errorf("truncatePrompt(long) should end with '...', got %q", got[77:])
	}
}
