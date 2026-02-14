package git

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type Git struct {
	Dir     string
	Verbose bool
}

func (g *Git) Run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if g.Dir != "" {
		cmd.Dir = g.Dir
	}

	if g.Verbose {
		if g.Dir != "" {
			fmt.Fprintf(os.Stderr, "$ git -C %s %s\n", g.Dir, strings.Join(args, " "))
		} else {
			fmt.Fprintf(os.Stderr, "$ git %s\n", strings.Join(args, " "))
		}
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out)), nil
}
