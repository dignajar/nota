// Package store keeps notes as one Markdown file per note on disk.
package store

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	ErrNotFound  = errors.New("note not found")
	ErrAmbiguous = errors.New("id prefix matches more than one note")
)

// Store is a directory of notes.
type Store struct {
	Dir string // data dir; notes live in Dir/notes
}

// DefaultDir resolves the data directory:
// $NOTA_HOME, else $XDG_DATA_HOME/nota, else ~/.local/share/nota.
func DefaultDir() (string, error) {
	if d := os.Getenv("NOTA_HOME"); d != "" {
		return filepath.Abs(d)
	}
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "nota"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "nota"), nil
}

// Open ensures the directory layout exists.
func Open(dir string) (*Store, error) {
	s := &Store{Dir: dir}
	if err := os.MkdirAll(s.NotesDir(), 0o700); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return s, nil
}

// NotesDir is where note files live.
func (s *Store) NotesDir() string { return filepath.Join(s.Dir, "notes") }

func (s *Store) pathFor(id string) string { return filepath.Join(s.NotesDir(), id+".md") }

func isNoteFile(name string) bool {
	return strings.HasSuffix(name, ".md") && !strings.HasPrefix(name, ".")
}

// LoadAll reads every note. Files that cannot be parsed are skipped and
// reported as warnings rather than failing the whole load.
func (s *Store) LoadAll() ([]*Note, []error) {
	entries, err := os.ReadDir(s.NotesDir())
	if err != nil {
		return nil, []error{err}
	}
	var notes []*Note
	var warns []error
	for _, e := range entries {
		if e.IsDir() || !isNoteFile(e.Name()) {
			continue
		}
		n, err := s.loadFile(filepath.Join(s.NotesDir(), e.Name()))
		if err != nil {
			warns = append(warns, err)
			continue
		}
		notes = append(notes, n)
	}
	return notes, warns
}

func (s *Store) loadFile(path string) (*Note, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	n, err := Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	n.ID = strings.TrimSuffix(filepath.Base(path), ".md") // file name is the source of truth
	n.Path = path
	if n.Created.IsZero() {
		if fi, err := os.Stat(path); err == nil {
			n.Created = fi.ModTime()
		}
	}
	return n, nil
}

// Get loads a note by exact id.
func (s *Store) Get(id string) (*Note, error) {
	n, err := s.loadFile(s.pathFor(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return n, err
}

// Find resolves an id or unique id prefix.
func (s *Store) Find(prefix string) (*Note, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	if prefix == "" {
		return nil, ErrNotFound
	}
	if n, err := s.Get(prefix); err == nil {
		return n, nil
	}
	entries, err := os.ReadDir(s.NotesDir())
	if err != nil {
		return nil, err
	}
	var match string
	for _, e := range entries {
		if !isNoteFile(e.Name()) {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		if strings.HasPrefix(strings.ToLower(id), prefix) {
			if match != "" {
				return nil, fmt.Errorf("%w: %q", ErrAmbiguous, prefix)
			}
			match = id
		}
	}
	if match == "" {
		return nil, fmt.Errorf("%w: %q", ErrNotFound, prefix)
	}
	return s.Get(match)
}

// Create assigns a fresh id and writes the note.
func (s *Store) Create(n *Note) error {
	if n.Created.IsZero() {
		n.Created = time.Now()
	}
	if n.Status == "" {
		n.Status = StatusOpen
	}
	for i := 0; i < 20; i++ {
		id := NewID()
		if _, err := os.Stat(s.pathFor(id)); errors.Is(err, os.ErrNotExist) {
			n.ID = id
			return s.Save(n)
		}
	}
	return errors.New("could not allocate a unique id")
}

// Save writes a note atomically: temp file in the same dir, fsync, rename.
func (s *Store) Save(n *Note) error {
	if n.ID == "" {
		return errors.New("note has no id")
	}
	path := s.pathFor(n.ID)
	tmp, err := os.CreateTemp(s.NotesDir(), ".tmp-"+n.ID+"-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after a successful rename
	if _, err := tmp.Write(Marshal(n)); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	n.Path = path
	return nil
}

// Update re-reads a note, applies fn, and saves it. Re-reading right before
// writing keeps the window for clobbering a concurrent edit small.
func (s *Store) Update(id string, fn func(*Note)) (*Note, error) {
	n, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	fn(n)
	return n, s.Save(n)
}

// Delete removes a note file.
func (s *Store) Delete(id string) error {
	err := os.Remove(s.pathFor(id))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}

// Signature is a cheap fingerprint of the notes dir (names, sizes, mtimes)
// used by the daemon to detect changes without reading every file.
func (s *Store) Signature() (string, error) {
	entries, err := os.ReadDir(s.NotesDir())
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range entries {
		if !isNoteFile(e.Name()) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		fmt.Fprintf(&b, "%s:%d:%d;", e.Name(), fi.Size(), fi.ModTime().UnixNano())
	}
	return b.String(), nil
}

// idAlphabet avoids look-alike characters (0/o, 1/l/i).
const idAlphabet = "23456789abcdefghjkmnpqrstuvwxyz"

// NewID returns a short random id such as "k3f9qx".
func NewID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // crypto/rand never fails on supported platforms
	}
	for i := range buf {
		buf[i] = idAlphabet[int(buf[i])%len(idAlphabet)]
	}
	return string(buf)
}

// Sort orders notes for display: open reminders by due time first, then the
// rest newest first.
func Sort(notes []*Note) {
	sort.SliceStable(notes, func(i, j int) bool {
		a, b := notes[i], notes[j]
		ar, br := a.HasReminder() && a.IsOpen(), b.HasReminder() && b.IsOpen()
		if ar != br {
			return ar
		}
		if ar && br && !a.RemindAt.Equal(b.RemindAt) {
			return a.RemindAt.Before(b.RemindAt)
		}
		return a.Created.After(b.Created)
	})
}
