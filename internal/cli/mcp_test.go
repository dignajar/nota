package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type mcpReply struct {
	ID     json.RawMessage `json:"id"`
	Result struct {
		ProtocolVersion string           `json:"protocolVersion"`
		Tools           []map[string]any `json:"tools"`
		Content         []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *rpcError `json:"error"`
}

func (r mcpReply) text() string {
	if len(r.Result.Content) == 0 {
		return ""
	}
	return r.Result.Content[0].Text
}

// mcpSession sends each message as one line to nota mcp and returns the replies.
func (h *harness) mcpSession(msgs ...string) []mcpReply {
	h.t.Helper()
	var out, errb bytes.Buffer
	a := &App{
		Out: &out, Err: &errb, In: strings.NewReader(strings.Join(msgs, "\n") + "\n"),
		Now: func() time.Time { return h.now }, Dir: h.dir, Cwd: h.cwd,
	}
	if code := a.Run([]string{"mcp"}); code != 0 {
		h.t.Fatalf("nota mcp: exit %d: %s", code, errb.String())
	}
	var replies []mcpReply
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var r mcpReply
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			h.t.Fatalf("reply is not one JSON line: %q: %v", line, err)
		}
		replies = append(replies, r)
	}
	return replies
}

// call is a tools/call request with id 1.
func call(name string, args any) string {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": name, "arguments": args}})
	return string(b)
}

func (h *harness) tool(name string, args any) mcpReply {
	h.t.Helper()
	r := h.mcpSession(call(name, args))
	if len(r) != 1 {
		h.t.Fatalf("%s: want 1 reply, got %d", name, len(r))
	}
	if r[0].Error != nil {
		h.t.Fatalf("%s: rpc error %+v", name, r[0].Error)
	}
	return r[0]
}

func TestMCPHandshake(t *testing.T) {
	h := newHarness(t)
	r := h.mcpSession(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":"b","method":"ping"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":4,"method":"initialize","params":{"protocolVersion":"1999-01-01"}}`,
	)
	if len(r) != 4 {
		t.Fatalf("notifications must not get a reply: got %d replies", len(r))
	}
	if r[0].Result.ProtocolVersion != "2025-06-18" {
		t.Fatalf("supported version must be echoed, got %q", r[0].Result.ProtocolVersion)
	}
	if string(r[1].ID) != `"b"` || r[1].Error != nil {
		t.Fatalf("ping: %+v", r[1])
	}
	names := map[string]bool{}
	for _, tl := range r[2].Result.Tools {
		names[tl["name"].(string)] = true
		if tl["inputSchema"].(map[string]any)["type"] != "object" {
			t.Fatalf("%v: inputSchema must be an object schema", tl["name"])
		}
	}
	for _, n := range []string{"add_note", "list_notes", "search_notes", "show_note", "done_notes", "reopen_notes", "delete_notes", "remind_note", "snooze_note"} {
		if !names[n] {
			t.Fatalf("missing tool %s in %v", n, names)
		}
	}
	if r[3].Result.ProtocolVersion != mcpVersions[0] {
		t.Fatalf("unknown version must get our latest, got %q", r[3].Result.ProtocolVersion)
	}
}

func TestMCPProtocolErrors(t *testing.T) {
	h := newHarness(t)
	r := h.mcpSession(
		`not json`,
		`{"jsonrpc":"2.0","id":2,"method":"nope"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"add_note","arguments":{"text":5}}}`,
		`{"id":5,"method":"ping"}`,
	)
	want := []int{rpcParseError, rpcMethodNotFound, rpcInvalidParams, rpcInvalidParams, rpcInvalidRequest}
	if len(r) != len(want) {
		t.Fatalf("want %d replies, got %d", len(want), len(r))
	}
	for i, code := range want {
		if r[i].Error == nil || r[i].Error.Code != code {
			t.Fatalf("reply %d: want error %d, got %+v", i, code, r[i])
		}
	}
	if string(r[0].ID) != "null" {
		t.Fatalf("parse error id must be null, got %s", r[0].ID)
	}
}

func TestMCPNoteLifecycle(t *testing.T) {
	h := newHarness(t)

	// text that looks like a flag stays text
	add := h.tool("add_note", map[string]any{"text": "-rf the cache #ops", "in": "2h", "tags": []string{"Prod"}})
	if add.Result.IsError || !strings.Contains(add.text(), "Added") || !strings.Contains(add.text(), "in 2h") {
		t.Fatalf("add_note: %+v", add)
	}
	if strings.Contains(add.text(), "\x1b[") {
		t.Fatalf("tool output must not be colored: %q", add.text())
	}
	h.tool("add_note", map[string]any{"text": "plain idea"})

	var notes []jsonNote
	list := h.tool("list_notes", map[string]any{"tags": []string{"ops"}})
	if err := json.Unmarshal([]byte(list.text()), &notes); err != nil {
		t.Fatalf("list_notes must return JSON: %v: %q", err, list.text())
	}
	if len(notes) != 1 || notes[0].Text != "-rf the cache #ops" || strings.Join(notes[0].Tags, ",") != "ops,prod" {
		t.Fatalf("list_notes: %+v", notes)
	}
	id := notes[0].ID

	if s := h.tool("show_note", map[string]any{"id": id[:4]}); !strings.Contains(s.text(), "-rf the cache") {
		t.Fatalf("show_note: %q", s.text())
	}
	if r := h.tool("snooze_note", map[string]any{"id": id, "duration": "30m"}); r.Result.IsError || !strings.Contains(r.text(), "in 30m") {
		t.Fatalf("snooze_note: %+v", r)
	}
	if r := h.tool("remind_note", map[string]any{"id": id, "when": "tomorrow 9am"}); r.Result.IsError {
		t.Fatalf("remind_note: %+v", r)
	}
	if r := h.tool("remind_note", map[string]any{"id": id, "when": "off"}); !strings.Contains(r.text(), "Cleared") {
		t.Fatalf("remind_note off: %+v", r)
	}
	if r := h.tool("done_notes", map[string]any{"ids": []string{id}}); r.Result.IsError {
		t.Fatalf("done_notes: %+v", r)
	}
	if got := h.jsonList(); len(got) != 1 || got[0].Text != "plain idea" {
		t.Fatalf("after done: %+v", got)
	}

	s := h.tool("search_notes", map[string]any{"query": "cache"})
	if err := json.Unmarshal([]byte(s.text()), &notes); err != nil || len(notes) != 1 || notes[0].Status != "done" {
		t.Fatalf("search_notes must find done notes: %v %q", err, s.text())
	}
	h.tool("reopen_notes", map[string]any{"ids": []string{id}})
	if r := h.tool("delete_notes", map[string]any{"ids": []string{id}}); !strings.Contains(r.text(), "Removed") {
		t.Fatalf("delete_notes: %+v", r)
	}
	if got := h.jsonList("--all"); len(got) != 1 {
		t.Fatalf("after delete: %+v", got)
	}
}

func TestMCPToolErrors(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		name string
		args map[string]any
		want string
	}{
		{"add_note", map[string]any{"text": "  "}, "text is required"},
		{"add_note", map[string]any{"text": "x", "in": "-h"}, "not a duration"},
		{"add_note", map[string]any{"text": "x", "in": "1h", "at": "9am"}, "either --in or --at"},
		{"add_note", map[string]any{"text": "x", "dir": "relative"}, "absolute"},
		{"list_notes", map[string]any{"status": "weird"}, "status must be"},
		{"search_notes", map[string]any{"query": "x", "status": "open"}, "status must be"},
		{"show_note", map[string]any{"id": "zzzz"}, "not found"},
		{"done_notes", map[string]any{"ids": []string{}}, "ids is required"},
		{"remind_note", map[string]any{"id": "zzzz"}, "when is required"},
	} {
		r := h.tool(tc.name, tc.args)
		if !r.Result.IsError || !strings.Contains(r.text(), tc.want) {
			t.Errorf("%s %v: want error containing %q, got %+v", tc.name, tc.args, tc.want, r)
		}
	}
	if got := h.jsonList("--all"); len(got) != 0 {
		t.Fatalf("failed calls must not create notes: %+v", got)
	}
}

func TestMCPDirContext(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	h := newHarness(t)
	repo := filepath.Join(t.TempDir(), "infra")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	h.tool("add_note", map[string]any{"text": "in the repo", "dir": repo})
	h.tool("add_note", map[string]any{"text": "elsewhere"})

	var notes []jsonNote
	r := h.tool("list_notes", map[string]any{"here": true, "dir": repo})
	if err := json.Unmarshal([]byte(r.text()), &notes); err != nil {
		t.Fatal(err)
	}
	if len(notes) != 1 || notes[0].Text != "in the repo" || notes[0].GitRepo != "infra" {
		t.Fatalf("here with dir: %+v", notes)
	}
	if r := h.tool("list_notes", map[string]any{"limit": 1}); !strings.HasPrefix(r.text(), "[") {
		t.Fatalf("limit: %q", r.text())
	}
	if n := len(h.jsonList()); n != 2 {
		t.Fatalf("want 2 notes, got %d", n)
	}
}
