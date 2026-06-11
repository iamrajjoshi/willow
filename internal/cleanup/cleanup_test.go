package cleanup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamrajjoshi/willow/internal/git"
)

func TestUpstreamStatusesDetectsGoneExistingAndNoUpstream(t *testing.T) {
	workDir := setupCleanupGitRepo(t)
	g := &git.Git{Dir: workDir}

	createAndPushBranch(t, g, workDir, "gone")
	createAndPushBranch(t, g, workDir, "existing")

	if _, err := g.Run("checkout", "-b", "local-only", "main"); err != nil {
		t.Fatalf("checkout local-only: %v", err)
	}
	writeCommit(t, g, workDir, "local-only.txt", "local only")

	if _, err := (&git.Git{Dir: filepath.Join(filepath.Dir(workDir), "origin.git")}).Run("update-ref", "-d", "refs/heads/gone"); err != nil {
		t.Fatalf("delete remote branch: %v", err)
	}
	if _, err := g.Run("fetch", "--prune", "origin"); err != nil {
		t.Fatalf("fetch --prune: %v", err)
	}

	statuses, err := UpstreamStatuses(g, []string{"gone", "existing", "local-only", "missing"})
	if err != nil {
		t.Fatalf("UpstreamStatuses: %v", err)
	}
	if !statuses["gone"].Gone {
		t.Fatalf("gone branch status = %#v, want gone", statuses["gone"])
	}
	if statuses["existing"].Gone {
		t.Fatalf("existing branch status = %#v, want not gone", statuses["existing"])
	}
	if statuses["local-only"].Gone || statuses["local-only"].Upstream != "" {
		t.Fatalf("local-only branch status = %#v, want no upstream and not gone", statuses["local-only"])
	}
	if _, ok := statuses["missing"]; ok {
		t.Fatalf("missing branch should not appear in statuses: %#v", statuses["missing"])
	}
}

func setupCleanupGitRepo(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	g := &git.Git{}
	if _, err := g.Run("init", "--bare", "--initial-branch=main", origin); err != nil {
		t.Fatalf("git init --bare: %v", err)
	}

	workDir := filepath.Join(root, "work")
	if _, err := g.Run("clone", origin, workDir); err != nil {
		t.Fatalf("git clone: %v", err)
	}
	wg := &git.Git{Dir: workDir}
	if _, err := wg.Run("config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("config email: %v", err)
	}
	if _, err := wg.Run("config", "user.name", "Test"); err != nil {
		t.Fatalf("config name: %v", err)
	}
	writeCommit(t, wg, workDir, "README.md", "initial")
	if _, err := wg.Run("push", "-u", "origin", "main"); err != nil {
		t.Fatalf("push main: %v", err)
	}
	return workDir
}

func createAndPushBranch(t *testing.T, g *git.Git, workDir, branch string) {
	t.Helper()

	if _, err := g.Run("checkout", "-b", branch, "main"); err != nil {
		t.Fatalf("checkout %s: %v", branch, err)
	}
	writeCommit(t, g, workDir, branch+".txt", branch)
	if _, err := g.Run("push", "-u", "origin", branch); err != nil {
		t.Fatalf("push %s: %v", branch, err)
	}
}

func writeCommit(t *testing.T, g *git.Git, dir, name, contents string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if _, err := g.Run("add", name); err != nil {
		t.Fatalf("git add %s: %v", name, err)
	}
	msg := "add " + strings.TrimSuffix(name, filepath.Ext(name))
	if _, err := g.Run("commit", "-m", msg); err != nil {
		t.Fatalf("git commit %s: %v", name, err)
	}
}
