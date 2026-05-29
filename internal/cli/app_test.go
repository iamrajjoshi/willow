package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveRepoFromGitMetadataCwdWillowWorktree(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("WILLOW_BASE_DIR", baseDir)

	bareDir := filepath.Join(baseDir, "repos", "repo.git")
	adminDir := filepath.Join(bareDir, "worktrees", "feature")
	wtDir := filepath.Join(baseDir, "worktrees", "repo", "feature")
	writeCLITestFile(t, filepath.Join(adminDir, "commondir"), "../..\n")
	writeCLITestFile(t, filepath.Join(wtDir, ".git"), "gitdir: "+adminDir+"\n")
	t.Chdir(wtDir)

	got, isWillow, foundGit := resolveRepoFromGitMetadataCwd()
	if !foundGit || !isWillow {
		t.Fatalf("resolveRepoFromGitMetadataCwd() foundGit=%v isWillow=%v, want true/true", foundGit, isWillow)
	}
	if got != filepath.Clean(bareDir) {
		t.Fatalf("bareDir = %q, want %q", got, filepath.Clean(bareDir))
	}
}

func TestResolveRepoFromGitMetadataCwdNonWillowGitRepo(t *testing.T) {
	baseDir := t.TempDir()
	t.Setenv("WILLOW_BASE_DIR", baseDir)

	repoDir := filepath.Join(t.TempDir(), "repo")
	cwd := filepath.Join(repoDir, "subdir")
	writeCLITestFile(t, filepath.Join(repoDir, ".git", "HEAD"), "ref: refs/heads/main\n")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir cwd: %v", err)
	}
	t.Chdir(cwd)

	_, isWillow, foundGit := resolveRepoFromGitMetadataCwd()
	if !foundGit || isWillow {
		t.Fatalf("resolveRepoFromGitMetadataCwd() foundGit=%v isWillow=%v, want true/false", foundGit, isWillow)
	}
}

func writeCLITestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
