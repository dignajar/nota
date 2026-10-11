// Package gitctx captures where a note was written: directory, repo, branch.
package gitctx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Context describes the working location.
type Context struct {
	Cwd    string
	Root   string // git toplevel, "" outside a repo
	Repo   string // basename of Root
	Branch string
}

// Detect inspects dir (or the current directory when empty). Git errors are
// ignored: outside a repo, or without git installed, only Cwd is set.
func Detect(dir string) Context {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	c := Context{Cwd: dir}
	if root := git(dir, "rev-parse", "--show-toplevel"); root != "" {
		c.Root = root
		c.Repo = filepath.Base(root)
		// symbolic-ref also works in a repo with no commits yet;
		// on a detached HEAD fall back to the short commit hash.
		c.Branch = git(dir, "symbolic-ref", "--short", "-q", "HEAD")
		if c.Branch == "" {
			c.Branch = git(dir, "rev-parse", "--short", "HEAD")
		}
	}
	return c
}

func git(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	done := make(chan struct{})
	var out []byte
	var err error
	go func() { out, err = cmd.Output(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second): // never let a slow repo block capture
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return ""
	}
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
