package termfmt

import (
	"strings"
	"testing"
)

func TestVisibleWidthIgnoresANSIAndCountsWideRunes(t *testing.T) {
	got := VisibleWidth("\033[31m✅ DONE\033[0m └─ branch")
	want := 17
	if got != want {
		t.Fatalf("VisibleWidth() = %d, want %d", got, want)
	}
}

func TestPaddingUsesVisibleWidth(t *testing.T) {
	got := PadRight("\033[2mabc\033[0m", 5)
	if VisibleWidth(got) != 5 {
		t.Fatalf("PadRight visible width = %d, want 5", VisibleWidth(got))
	}
	if !strings.HasSuffix(got, "  ") {
		t.Fatalf("PadRight() = %q, want two visible spaces at end", got)
	}

	got = PadLeft("✅", 4)
	if VisibleWidth(got) != 4 {
		t.Fatalf("PadLeft visible width = %d, want 4", VisibleWidth(got))
	}
	if !strings.HasPrefix(got, "  ") {
		t.Fatalf("PadLeft() = %q, want two visible spaces at start", got)
	}
}

func TestTruncateEndAddsEllipsisAtWidth(t *testing.T) {
	got := TruncateEnd("raj--tprm-464--backend-validate-review-risk-subtype", 18)
	if got != "raj--tprm-464--ba…" {
		t.Fatalf("TruncateEnd() = %q", got)
	}
	if VisibleWidth(got) != 18 {
		t.Fatalf("TruncateEnd visible width = %d, want 18", VisibleWidth(got))
	}
}

func TestTruncateEndPreservesANSI(t *testing.T) {
	got := TruncateEnd("\033[2mabcdef\033[0m", 4)
	if !strings.Contains(got, "\033[2m") || !strings.HasSuffix(got, "\033[0m") {
		t.Fatalf("TruncateEnd() should preserve ANSI wrapping, got %q", got)
	}
	if VisibleWidth(got) != 4 {
		t.Fatalf("TruncateEnd visible width = %d, want 4", VisibleWidth(got))
	}
}

func TestShortenHome(t *testing.T) {
	got := ShortenHome("/Users/raj.joshi/.willow/worktrees/repo/main", "/Users/raj.joshi")
	want := "~/.willow/worktrees/repo/main"
	if got != want {
		t.Fatalf("ShortenHome() = %q, want %q", got, want)
	}
}
