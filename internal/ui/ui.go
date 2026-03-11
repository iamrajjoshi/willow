package ui

import (
	"fmt"
	"os"
)

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	dim    = "\033[2m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
)

type UI struct{}

func (u *UI) Success(msg string) {
	fmt.Printf("%s %s\n", u.Green("✔"), msg)
}

func (u *UI) Info(msg string) {
	fmt.Println(msg)
}

func (u *UI) Warn(msg string) {
	fmt.Printf("%s %s\n", u.Yellow("⚠"), msg)
}

func (u *UI) Errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s "+format+"\n", append([]any{u.Red("error:")}, args...)...)
}

func (u *UI) Bold(s string) string {
	return bold + s + reset
}

func (u *UI) Green(s string) string {
	return green + s + reset
}

func (u *UI) Yellow(s string) string {
	return yellow + s + reset
}

func (u *UI) Red(s string) string {
	return red + s + reset
}

func (u *UI) Cyan(s string) string {
	return cyan + s + reset
}

func (u *UI) Dim(s string) string {
	return dim + s + reset
}

// ANSI terminal control sequences for TUI rendering
func CursorHome() string    { return "\033[H" }
func ClearToEnd() string    { return "\033[J" }
func HideCursor() string    { return "\033[?25l" }
func ShowCursor() string    { return "\033[?25h" }
func AltScreenOn() string   { return "\033[?1049h" }
func AltScreenOff() string  { return "\033[?1049l" }
