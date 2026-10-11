package store

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Marshal renders a note as Markdown with YAML front matter.
// Only a small, fixed YAML subset is emitted so the file stays readable and
// valid YAML without pulling in a YAML library.
func Marshal(n *Note) []byte {
	var b bytes.Buffer
	b.WriteString("---\n")
	writeKV(&b, "id", n.ID)
	writeKV(&b, "created", formatTime(n.Created))
	writeKV(&b, "remind_at", formatTime(n.RemindAt))
	status := n.Status
	if status == "" {
		status = StatusOpen
	}
	writeKV(&b, "status", string(status))
	b.WriteString("notified: " + strconv.FormatBool(n.Notified) + "\n")
	b.WriteString("tags: [" + strings.Join(NormalizeTags(n.Tags), ", ") + "]\n")
	writeKV(&b, "cwd", n.Cwd)
	writeKV(&b, "git_root", n.GitRoot)
	writeKV(&b, "git_repo", n.GitRepo)
	writeKV(&b, "git_branch", n.GitBranch)
	for _, kv := range n.Extra {
		b.WriteString(kv[0] + ": " + kv[1] + "\n") // raw, as found in the file
	}
	b.WriteString("---\n\n")
	b.WriteString(strings.TrimSpace(n.Body))
	b.WriteString("\n")
	return b.Bytes()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func writeKV(b *bytes.Buffer, key, val string) {
	b.WriteString(key)
	b.WriteString(":")
	if val != "" {
		b.WriteString(" ")
		b.WriteString(quoteIfNeeded(val))
	}
	b.WriteString("\n")
}

func quoteIfNeeded(s string) string {
	plain := true
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/' || r == '.' || r == '_' || r == '+':
		case r == '-' && i > 0:
		case r == ':' && i > 0 && i < len(s)-1 && s[i+1] != ' ':
		default:
			plain = false
		}
	}
	if plain && s != "true" && s != "false" && s != "null" && s != "~" {
		return s
	}
	var q strings.Builder
	q.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			q.WriteString(`\\`)
		case '"':
			q.WriteString(`\"`)
		case '\n':
			q.WriteString(`\n`)
		case '\t':
			q.WriteString(`\t`)
		case '\r':
			q.WriteString(`\r`)
		default:
			q.WriteRune(r)
		}
	}
	q.WriteByte('"')
	return q.String()
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return strings.ReplaceAll(s[1:len(s)-1], "''", "'")
	}
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		// strip a trailing " # comment" from plain scalars
		if i := strings.Index(s, " #"); i >= 0 {
			s = strings.TrimSpace(s[:i])
		}
		return s
	}
	in := s[1 : len(s)-1]
	var out strings.Builder
	for i := 0; i < len(in); i++ {
		c := in[i]
		if c == '\\' && i+1 < len(in) {
			i++
			switch in[i] {
			case 'n':
				out.WriteByte('\n')
			case 't':
				out.WriteByte('\t')
			case 'r':
				out.WriteByte('\r')
			default:
				out.WriteByte(in[i])
			}
			continue
		}
		out.WriteByte(c)
	}
	return out.String()
}

func parseList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = unquote(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", s)
}

// Unmarshal parses a note file. A file without front matter is accepted and
// treated as a plain open note whose whole content is the body, so you can
// drop any .md file into the notes folder.
func Unmarshal(data []byte) (*Note, error) {
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	n := &Note{Status: StatusOpen}

	if !strings.HasPrefix(text, "---\n") {
		n.Body = strings.TrimSpace(text)
		return n, nil
	}
	rest := text[4:]
	end := strings.Index(rest, "\n---\n")
	var header string
	switch {
	case end >= 0:
		header, n.Body = rest[:end], rest[end+5:]
	case strings.HasSuffix(rest, "\n---"):
		header = strings.TrimSuffix(rest, "\n---")
	case strings.HasPrefix(rest, "---\n") || rest == "---":
		header, n.Body = "", strings.TrimPrefix(strings.TrimPrefix(rest, "---"), "\n")
	default:
		return nil, fmt.Errorf("front matter is not closed with ---")
	}
	n.Body = strings.TrimSpace(n.Body)

	for i, line := range strings.Split(header, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("line %d: expected key: value", i+2)
		}
		key = strings.TrimSpace(key)
		raw := strings.TrimSpace(val)
		v := unquote(raw)
		var err error
		switch key {
		case "id":
			n.ID = v
		case "created":
			n.Created, err = parseTime(v)
		case "remind_at":
			n.RemindAt, err = parseTime(v)
		case "status":
			switch Status(strings.ToLower(v)) {
			case StatusDone:
				n.Status = StatusDone
			case StatusOpen, "":
				n.Status = StatusOpen
			default:
				err = fmt.Errorf("unknown status %q", v)
			}
		case "notified":
			n.Notified, err = strconv.ParseBool(strings.ToLower(v))
			if v == "" {
				n.Notified, err = false, nil
			}
		case "tags":
			n.Tags = NormalizeTags(parseList(raw))
		case "cwd":
			n.Cwd = v
		case "git_root":
			n.GitRoot = v
		case "git_repo":
			n.GitRepo = v
		case "git_branch":
			n.GitBranch = v
		default:
			n.Extra = append(n.Extra, [2]string{key, raw})
		}
		if err != nil {
			return nil, fmt.Errorf("line %d (%s): %w", i+2, key, err)
		}
	}
	return n, nil
}
