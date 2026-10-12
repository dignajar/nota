package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	created := time.Date(2026, 10, 12, 14, 30, 5, 0, time.Local)
	in := &Note{
		ID:        "abc234",
		Created:   created,
		RemindAt:  created.Add(2 * time.Hour),
		Status:    StatusOpen,
		Tags:      []string{"db", "Mongo"},
		Cwd:       "/home/diego/my repo: weird \"path\"",
		GitRoot:   "/home/diego/api",
		GitRepo:   "api",
		GitBranch: "feature/x",
		Body:      "check MongoDB indexes\n\nsecond line",
		Extra:     [][2]string{{"priority", "high"}},
	}
	out, err := Unmarshal(Marshal(in))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Created.Equal(in.Created) || !out.RemindAt.Equal(in.RemindAt) {
		t.Fatalf("times differ: %v %v", out.Created, out.RemindAt)
	}
	if out.Cwd != in.Cwd || out.GitBranch != in.GitBranch || out.Body != in.Body {
		t.Fatalf("fields differ:\n%#v", out)
	}
	if !reflect.DeepEqual(out.Tags, []string{"db", "mongo"}) {
		t.Fatalf("tags: %v", out.Tags)
	}
	if len(out.Extra) != 1 || out.Extra[0][0] != "priority" {
		t.Fatalf("extra keys lost: %v", out.Extra)
	}
}

func TestUnmarshalPlainMarkdown(t *testing.T) {
	n, err := Unmarshal([]byte("just a dropped file\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n.Body != "just a dropped file" || n.Status != StatusOpen {
		t.Fatalf("got %#v", n)
	}
}

func TestUnmarshalMalformed(t *testing.T) {
	if _, err := Unmarshal([]byte("---\nstatus: maybe\n---\nx")); err == nil {
		t.Fatal("expected error for bad status")
	}
	if _, err := Unmarshal([]byte("---\nid: x\nno close")); err == nil {
		t.Fatal("expected error for unclosed front matter")
	}
}

func TestStoreCRUDAndFind(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := &Note{Body: "first"}
	if err := s.Create(a); err != nil {
		t.Fatal(err)
	}
	got, err := s.Find(a.ID[:3])
	if err != nil && !errors.Is(err, ErrAmbiguous) {
		t.Fatal(err)
	}
	if err == nil && got.Body != "first" {
		t.Fatalf("got %q", got.Body)
	}
	if _, err := s.Update(a.ID, func(n *Note) { n.Status = StatusDone }); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Get(a.ID)
	if got.IsOpen() {
		t.Fatal("expected done")
	}

	// a malformed file is skipped with a warning, not fatal
	os.WriteFile(filepath.Join(s.NotesDir(), "broken.md"), []byte("---\nstatus: ???\n---\n"), 0o600)
	notes, warns := s.LoadAll()
	if len(notes) != 1 || len(warns) != 1 {
		t.Fatalf("notes=%d warns=%d", len(notes), len(warns))
	}

	// leftover temp files are ignored
	entries, _ := os.ReadDir(s.NotesDir())
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}

	if err := s.Delete(a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find(a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestFindAmbiguous(t *testing.T) {
	s, _ := Open(t.TempDir())
	for _, id := range []string{"aaa222", "aab333"} {
		if err := s.Save(&Note{ID: id, Body: id}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Find("aa"); !errors.Is(err, ErrAmbiguous) {
		t.Fatalf("expected ambiguous, got %v", err)
	}
	if n, err := s.Find("aab"); err != nil || n.ID != "aab333" {
		t.Fatalf("got %v %v", n, err)
	}
}

func TestNormalizeTagsSplitsLists(t *testing.T) {
	got := NormalizeTags([]string{"aws,cost", "#Ops K8s", "aws", ""})
	want := []string{"aws", "cost", "k8s", "ops"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestExtractTags(t *testing.T) {
	got := ExtractTags("look into #bug in #API, not issue #123 or a#b. #ops/k8s.")
	want := []string{"api", "bug", "ops/k8s"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestQuotingRoundTrip(t *testing.T) {
	values := []string{
		"plain", "with space", "true", "null", "~", "-leading", "a: b", "a:b",
		"trailing:", "#hash", "quote\"inside", `back\slash`, "multi\nline", "tab\tchar",
		"ünïcödé ✓", "/path/with spaces/x", "feature/x-1.2",
	}
	for _, v := range values {
		in := &Note{ID: "id1234", Status: StatusOpen, Cwd: v, GitBranch: v, Body: "b"}
		out, err := Unmarshal(Marshal(in))
		if err != nil {
			t.Errorf("%q: %v", v, err)
			continue
		}
		if out.Cwd != v || out.GitBranch != v {
			t.Errorf("%q came back as cwd=%q branch=%q", v, out.Cwd, out.GitBranch)
		}
	}
}

func TestBodyContainingFrontMatterDelimiter(t *testing.T) {
	in := &Note{ID: "abc234", Status: StatusOpen, Body: "before\n---\nafter: not a key\n---\nend"}
	out, err := Unmarshal(Marshal(in))
	if err != nil {
		t.Fatal(err)
	}
	if out.Body != in.Body || out.ID != "abc234" {
		t.Fatalf("body mangled: %q", out.Body)
	}
}

func TestLoadAllSkipsCorruptFiles(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Create(&Note{Body: "good"}); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(s.NotesDir(), "broken.md")
	if err := os.WriteFile(bad, []byte("---\nstatus: maybe\n---\nx"), 0o644); err != nil {
		t.Fatal(err)
	}
	// non-note files must be ignored, not reported
	os.WriteFile(filepath.Join(s.NotesDir(), "README.txt"), []byte("x"), 0o644)

	notes, errs := s.LoadAll()
	if len(notes) != 1 || notes[0].Body != "good" {
		t.Fatalf("notes: %+v", notes)
	}
	if len(errs) != 1 {
		t.Fatalf("expected exactly the broken file reported, got %v", errs)
	}
}

func TestSignatureChangesWithContent(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	n := &Note{Body: "one"}
	if err := s.Create(n); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Signature()
	again, _ := s.Signature()
	if before != again {
		t.Fatal("signature must be stable when nothing changed")
	}
	if _, err := s.Update(n.ID, func(n *Note) { n.Body = "one, but longer" }); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Signature()
	if after == before {
		t.Fatal("signature did not change after an edit")
	}
}

func TestSortOrder(t *testing.T) {
	now := time.Date(2026, 10, 12, 10, 0, 0, 0, time.UTC)
	notes := []*Note{
		{ID: "old", Status: StatusOpen, Created: now.Add(-48 * time.Hour)},
		{ID: "late", Status: StatusOpen, Created: now, RemindAt: now.Add(5 * time.Hour)},
		{ID: "new", Status: StatusOpen, Created: now},
		{ID: "soon", Status: StatusOpen, Created: now, RemindAt: now.Add(time.Hour)},
	}
	Sort(notes)
	var got []string
	for _, n := range notes {
		got = append(got, n.ID)
	}
	if strings.Join(got, ",") != "soon,late,new,old" {
		t.Fatalf("order: %v", got)
	}
}

func TestNewIDShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		id := NewID()
		if len(id) != 6 || strings.ContainsAny(id, "01ilo") {
			t.Fatalf("bad id %q", id)
		}
		seen[id] = true
	}
	if len(seen) < 190 {
		t.Fatalf("ids collide too often: %d unique of 200", len(seen))
	}
}
