package gitctx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir,
		"-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func needGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
}

func TestDetectOutsideRepo(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	c := Detect(dir)
	if c.Cwd != dir || c.Root != "" || c.Repo != "" || c.Branch != "" {
		t.Fatalf("outside repo: %+v", c)
	}
}

func TestDetectRepoWithoutCommits(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "trunk")
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	c := Detect(sub)
	if c.Branch != "trunk" || c.Repo != filepath.Base(c.Root) || c.Cwd != sub {
		t.Fatalf("fresh repo: %+v", c)
	}
}

func TestDetectDetachedHead(t *testing.T) {
	needGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q", "-b", "main")
	gitRun(t, dir, "commit", "-q", "--allow-empty", "-m", "first")
	gitRun(t, dir, "checkout", "-q", "--detach")
	c := Detect(dir)
	if c.Branch == "" || c.Branch == "main" {
		t.Fatalf("detached HEAD should report a short hash, got %q", c.Branch)
	}
}
