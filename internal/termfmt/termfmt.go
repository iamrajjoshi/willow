package termfmt

import (
	"os"
	"strings"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

const DefaultWidth = 120

func TerminalWidth() int {
	if width, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && width > 0 {
		return width
	}
	if width, _, err := term.GetSize(int(os.Stderr.Fd())); err == nil && width > 0 {
		return width
	}
	return DefaultWidth
}

func Width(width int) int {
	if width > 0 {
		return width
	}
	return DefaultWidth
}

func ShortenHome(path, home string) string {
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func VisibleWidth(s string) int {
	return runewidth.StringWidth(StripANSI(s))
}

func PadRight(s string, width int) string {
	padding := width - VisibleWidth(s)
	if padding <= 0 {
		return s
	}
	return s + strings.Repeat(" ", padding)
}

func PadLeft(s string, width int) string {
	padding := width - VisibleWidth(s)
	if padding <= 0 {
		return s
	}
	return strings.Repeat(" ", padding) + s
}

func FitRight(s string, width int) string {
	return PadRight(TruncateEnd(s, width), width)
}

func FitLeft(s string, width int) string {
	return PadLeft(TruncateEnd(s, width), width)
}

func TruncateEnd(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if VisibleWidth(s) <= width {
		return s
	}

	ellipsis := "…"
	ellipsisW := runewidth.StringWidth(ellipsis)
	if width <= ellipsisW {
		return ellipsis
	}

	limit := width - ellipsisW
	var b strings.Builder
	used := 0
	hasANSI := false
	for i := 0; i < len(s); {
		if seq, next, ok := ansiSequence(s, i); ok {
			b.WriteString(seq)
			hasANSI = true
			i = next
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runewidth.RuneWidth(r)
		if used+rw > limit {
			break
		}
		b.WriteRune(r)
		used += rw
		i += size
	}
	b.WriteString(ellipsis)
	if hasANSI {
		b.WriteString("\033[0m")
	}
	return b.String()
}

func StripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if _, next, ok := ansiSequence(s, i); ok {
			i = next
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

func ansiSequence(s string, i int) (string, int, bool) {
	if i+1 >= len(s) || s[i] != '\x1b' || s[i+1] != '[' {
		return "", i, false
	}
	next := i + 2
	for next < len(s) {
		c := s[next]
		next++
		if c >= 0x40 && c <= 0x7e {
			return s[i:next], next, true
		}
	}
	return s[i:], len(s), true
}
