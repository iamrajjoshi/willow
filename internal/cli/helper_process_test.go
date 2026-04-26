package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("WILLOW_TEST_HELPER_PROCESS") == "willow" {
		willowTestHelperProcess()
		return
	}
	os.Exit(m.Run())
}

func willowTestHelperProcess() {
	logPath := os.Getenv("WILLOW_TEST_HELPER_LOG")
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintln(f, strings.Join(os.Args[1:], " "))
			_ = f.Close()
		}
	}
	if len(os.Args) < 2 {
		os.Exit(2)
	}

	switch os.Args[1] {
	case "new", "checkout":
		repo := "repo"
		for i, arg := range os.Args[2:] {
			if arg == "--repo" && i+3 < len(os.Args) {
				repo = os.Args[i+3]
			}
		}
		branch := os.Args[len(os.Args)-1]
		root := os.Getenv("WILLOW_TEST_HELPER_WT_ROOT")
		if root == "" {
			root = filepath.Join(os.TempDir(), "willow-helper-worktrees")
		}
		path := filepath.Join(root, repo, branch)
		_ = os.MkdirAll(path, 0o755)
		fmt.Println(path)
		os.Exit(0)
	case "rm":
		os.Exit(0)
	default:
		os.Exit(2)
	}
}
