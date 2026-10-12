package notify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCustomCommandReceivesTitleAndBody(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.txt")
	t.Setenv("NOTA_NOTIFY_CMD", `printf '%s|%s' "$NOTA_TITLE" "$NOTA_BODY" > "`+out+`"`)
	n := New()
	if !strings.Contains(n.Describe(), "NOTA_NOTIFY_CMD") {
		t.Fatalf("describe: %q", n.Describe())
	}
	if err := n.Send("Reminder", "check the rollout; it's \"flaky\""); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `Reminder|check the rollout; it's "flaky"` {
		t.Fatalf("got %q", got)
	}
}

func TestCustomCommandFailureIsReported(t *testing.T) {
	t.Setenv("NOTA_NOTIFY_CMD", "echo boom >&2; exit 3")
	err := New().Send("t", "b")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error containing command output, got %v", err)
	}
}
