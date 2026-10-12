package store

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

// Status of a note.
type Status string

const (
	StatusOpen Status = "open"
	StatusDone Status = "done"
)

// Note is a single captured item. It is stored as one Markdown file with
// YAML front matter. The file name (without .md) is the note ID.
type Note struct {
	ID        string
	Created   time.Time
	RemindAt  time.Time // zero means no reminder
	Status    Status
	Notified  bool
	Tags      []string
	Cwd       string
	GitRoot   string
	GitRepo   string
	GitBranch string
	Body      string

	// Extra keeps unknown front matter keys (in order) so hand edits survive rewrites.
	Extra [][2]string
	// Path is the file this note was loaded from. Not serialized.
	Path string
}

// HasReminder reports whether a reminder time is set.
func (n *Note) HasReminder() bool { return !n.RemindAt.IsZero() }

// IsOpen reports whether the note is not done.
func (n *Note) IsOpen() bool { return n.Status != StatusDone }

// Title is the first non empty line of the body.
func (n *Note) Title() string {
	for _, line := range strings.Split(n.Body, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return "(empty)"
}

// Context returns a short "repo@branch" or directory label, or "".
func (n *Note) Context() string {
	if n.GitRepo != "" {
		if n.GitBranch != "" && n.GitBranch != "HEAD" {
			return n.GitRepo + "@" + n.GitBranch
		}
		return n.GitRepo
	}
	return ""
}

var tagRe = regexp.MustCompile(`(?:^|[\s(\[,])#([\p{L}_][\p{L}\p{N}_\-/.]*)`)

// ExtractTags finds #tags in text, lowercased and without duplicates.
func ExtractTags(text string) []string {
	var tags []string
	for _, m := range tagRe.FindAllStringSubmatch(text, -1) {
		tags = append(tags, strings.TrimRight(m[1], "./-"))
	}
	return NormalizeTags(tags)
}

var tagCleanRe = regexp.MustCompile(`[^\p{L}\p{N}_\-/.]`)

// NormalizeTags lowercases, strips a leading '#', removes characters that
// would need quoting, drops empties and duplicates, and sorts.
func NormalizeTags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, raw := range tags {
		// "-t aws,cost" and "-t 'aws cost'" mean two tags
		for _, t := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			t = strings.ToLower(strings.TrimPrefix(t, "#"))
			t = tagCleanRe.ReplaceAllString(t, "")
			if t == "" || seen[t] {
				continue
			}
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

// HasTag reports whether the note has the given tag (case insensitive).
func (n *Note) HasTag(tag string) bool {
	tag = strings.ToLower(strings.TrimPrefix(tag, "#"))
	for _, t := range n.Tags {
		if t == tag {
			return true
		}
	}
	return false
}
