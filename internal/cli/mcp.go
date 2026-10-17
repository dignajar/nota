package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// nota mcp is a Model Context Protocol server on stdin/stdout, so editors
// and agents (Claude Code, Cursor, Claude Desktop) can manage notes. Each
// message is one JSON-RPC 2.0 object per line. Every tool runs the same code
// path as the CLI, so both behave the same.

// mcpVersions are the protocol revisions we accept, newest first.
var mcpVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const mcpInstructions = `nota stores short notes and reminders for the user. Use add_note to capture
a thought or a reminder ("in" such as 2h, or "at" such as "tomorrow 9am"); the
user gets a desktop notification when it is due. Pass "dir" with the absolute
path of the project you are working in so notes are attached to that repo, and
use list_notes with here=true to see what the user left for that repo.`

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

const (
	rpcParseError     = -32700
	rpcInvalidRequest = -32600
	rpcMethodNotFound = -32601
	rpcInvalidParams  = -32602
)

func cmdMCP(a *App, p *parsed) error {
	if len(p.args) > 0 {
		return usagef("unexpected argument %q", p.args[0])
	}
	s, err := a.store()
	if err != nil {
		return err
	}
	srv := &mcpServer{app: a, dataDir: s.Dir}
	return srv.serve(a.In, a.Out)
}

type mcpServer struct {
	app     *App
	dataDir string
}

func (m *mcpServer) serve(in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	enc := json.NewEncoder(out)
	for {
		line, err := r.ReadBytes('\n')
		if line = bytes.TrimSpace(line); len(line) > 0 {
			if resp := m.handle(line); resp != nil {
				if werr := enc.Encode(resp); werr != nil {
					return werr
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// handle answers one message. It returns nil for notifications, which never
// get a response.
func (m *mcpServer) handle(line []byte) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: json.RawMessage("null"),
			Error: &rpcError{rpcParseError, "parse error: " + err.Error()}}
	}
	isNotification := len(req.ID) == 0 || string(req.ID) == "null"
	if req.JSONRPC != "2.0" || req.Method == "" {
		if isNotification {
			return nil
		}
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{rpcInvalidRequest, "invalid request"}}
	}
	if isNotification {
		return nil // notifications/initialized, notifications/cancelled, ...
	}
	result, rerr := m.dispatch(req.Method, req.Params)
	if rerr != nil {
		return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: rerr}
	}
	return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (m *mcpServer) dispatch(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return nil, &rpcError{rpcInvalidParams, err.Error()}
			}
		}
		version := mcpVersions[0]
		for _, v := range mcpVersions {
			if v == p.ProtocolVersion {
				version = v
			}
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "nota", "version": Version},
			"instructions":    mcpInstructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{rpcInvalidParams, err.Error()}
		}
		return m.callTool(p.Name, p.Arguments)
	}
	return nil, &rpcError{rpcMethodNotFound, "method not found: " + method}
}

// toolArgs is the union of every tool's arguments.
type toolArgs struct {
	Text      string   `json:"text"`
	In        string   `json:"in"`
	At        string   `json:"at"`
	Tags      []string `json:"tags"`
	NoContext bool     `json:"no_context"`
	Dir       string   `json:"dir"`
	Status    string   `json:"status"`
	Due       bool     `json:"due"`
	Here      bool     `json:"here"`
	Limit     int      `json:"limit"`
	Query     string   `json:"query"`
	ID        string   `json:"id"`
	IDs       []string `json:"ids"`
	When      string   `json:"when"`
	Duration  string   `json:"duration"`
}

// callTool turns a tool call into nota CLI arguments. Values are passed as
// --flag=value and positional arguments after "--", so text that looks like
// a flag is never read as one.
func (m *mcpServer) callTool(name string, raw json.RawMessage) (any, *rpcError) {
	var t toolArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &t); err != nil {
			return nil, &rpcError{rpcInvalidParams, "invalid arguments: " + err.Error()}
		}
	}
	flag := func(args []string, name, val string) []string {
		if val != "" {
			args = append(args, "--"+name+"="+val)
		}
		return args
	}
	listArgs := func(cmd string, defaultStatus string) ([]string, error) {
		args := []string{cmd, "--json"}
		status := t.Status
		if status == "" {
			status = defaultStatus
		}
		switch status {
		case "open":
		case "done":
			args = append(args, "--done")
		case "all":
			args = append(args, "--all")
		default:
			return nil, fmt.Errorf(`status must be "open", "done" or "all"`)
		}
		if t.Due {
			args = append(args, "--due")
		}
		if t.Here {
			args = append(args, "--here")
		}
		for _, tag := range t.Tags {
			args = flag(args, "tag", tag)
		}
		if t.Limit < 0 {
			return nil, fmt.Errorf("limit must be positive")
		}
		if t.Limit > 0 {
			args = flag(args, "limit", strconv.Itoa(t.Limit))
		}
		return args, nil
	}
	need := func(field, val string) error {
		if strings.TrimSpace(val) == "" {
			return fmt.Errorf("%s is required", field)
		}
		return nil
	}

	var args []string
	var err error
	switch name {
	case "add_note":
		if err = need("text", t.Text); err == nil {
			args = flag(flag([]string{"add"}, "in", t.In), "at", t.At)
			for _, tag := range t.Tags {
				args = flag(args, "tag", tag)
			}
			if t.NoContext {
				args = append(args, "--no-context")
			}
			args = append(args, "--", t.Text)
		}
	case "list_notes":
		args, err = listArgs("list", "open")
	case "search_notes":
		if err = need("query", t.Query); err == nil {
			// search includes done notes unless asked for open or done only
			if args, err = listArgs("search", "all"); err == nil && t.Status == "open" {
				err = fmt.Errorf(`search_notes status must be "done" or "all"`)
			}
			args = append(args, "--", t.Query)
		}
	case "show_note":
		if err = need("id", t.ID); err == nil {
			args = []string{"show", "--", t.ID}
		}
	case "done_notes", "reopen_notes", "delete_notes":
		if len(t.IDs) == 0 {
			err = fmt.Errorf("ids is required")
		}
		cmd := map[string]string{"done_notes": "done", "reopen_notes": "reopen", "delete_notes": "rm"}[name]
		args = append([]string{cmd, "--"}, t.IDs...)
	case "remind_note":
		if err = need("id", t.ID); err == nil {
			if err = need("when", t.When); err == nil {
				args = []string{"remind", "--", t.ID, t.When}
			}
		}
	case "snooze_note":
		if err = need("id", t.ID); err == nil {
			args = []string{"snooze", "--", t.ID}
			if t.Duration != "" {
				args = append(args, t.Duration)
			}
		}
	default:
		return nil, &rpcError{rpcInvalidParams, "unknown tool: " + name}
	}
	if err == nil && t.Dir != "" {
		err = checkDir(t.Dir)
	}
	if err != nil {
		return toolResult(err.Error(), true), nil
	}
	return m.runCLI(args, t.Dir), nil
}

func checkDir(dir string) error {
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("dir must be an absolute path: %q", dir)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return fmt.Errorf("dir is not a directory: %q", dir)
	}
	return nil
}

// runCLI runs one nota command with captured, uncolored output.
func (m *mcpServer) runCLI(args []string, dir string) map[string]any {
	var out, errb bytes.Buffer
	sub := &App{
		Out: &out, Err: &errb,
		In: strings.NewReader(""), StdinIsTTY: true, // never wait on stdin
		Now: m.app.Now,
		Dir: m.dataDir,
		Cwd: m.app.Cwd,

		NoPositions: true,
	}
	if dir != "" {
		sub.Cwd = dir
	}
	code := sub.Run(args)
	text := strings.TrimSpace(out.String())
	if e := strings.TrimSpace(errb.String()); e != "" {
		if text != "" {
			text += "\n"
		}
		text += e
	}
	if text == "" {
		text = "ok"
	}
	return toolResult(text, code != 0)
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

// ---- tool definitions ----

type prop = map[string]any

func schema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func tool(name, title, desc string, readOnly, destructive bool, input map[string]any) map[string]any {
	return map[string]any{
		"name": name, "title": title, "description": desc, "inputSchema": input,
		"annotations": map[string]any{
			"title": title, "readOnlyHint": readOnly, "destructiveHint": destructive, "openWorldHint": false,
		},
	}
}

var (
	pDir  = prop{"type": "string", "description": "absolute path of the directory or repo the note belongs to (default: the server's working directory)"}
	pTags = prop{"type": "array", "items": prop{"type": "string"}, "description": "tags without #"}
	pID   = prop{"type": "string", "description": "note id or a unique prefix of it"}
	pIDs  = prop{"type": "array", "items": prop{"type": "string"}, "minItems": 1, "description": "note ids or unique prefixes"}
)

func listProps(statusDesc string) map[string]any {
	return map[string]any{
		"status": prop{"type": "string", "enum": []string{"open", "done", "all"}, "description": statusDesc},
		"due":    prop{"type": "boolean", "description": "only open notes with a reminder"},
		"tags":   prop{"type": "array", "items": prop{"type": "string"}, "description": "only notes with all these tags"},
		"here":   prop{"type": "boolean", "description": "only notes captured in dir's git repo (or dir itself outside a repo)"},
		"dir":    pDir,
		"limit":  prop{"type": "integer", "minimum": 1, "description": "maximum number of notes"},
	}
}

var mcpTools = []map[string]any{
	tool("add_note", "Add note",
		"Capture a note, optionally with a reminder. The user gets a desktop notification when the reminder is due. #tags in the text become tags. Returns the new note id.",
		false, false, schema([]string{"text"}, map[string]any{
			"text":       prop{"type": "string", "description": "note text; the first line is the title"},
			"in":         prop{"type": "string", "description": "remind after a duration: 30m, 2h, 1h30m, 2d"},
			"at":         prop{"type": "string", "description": `remind at a time: "tomorrow 9am", "fri 14:30", "eod", "2026-10-20 14:00"`},
			"tags":       pTags,
			"dir":        pDir,
			"no_context": prop{"type": "boolean", "description": "do not record the directory and git repo"},
		})),
	tool("list_notes", "List notes",
		"List notes as JSON, open reminders first by due time, then newest first.",
		true, false, schema(nil, listProps(`which notes to include (default "open")`))),
	tool("search_notes", "Search notes",
		"Find notes whose text, tags, repo or branch contain all the words of the query. Returns JSON.",
		true, false, schema([]string{"query"}, func() map[string]any {
			p := listProps(`"all" (default) or "done"`)
			p["status"] = prop{"type": "string", "enum": []string{"done", "all"}, "description": `"all" (default) or "done"`}
			p["query"] = prop{"type": "string", "description": "words that must all appear"}
			return p
		}())),
	tool("show_note", "Show note", "Show one note with its details and full text.",
		true, false, schema([]string{"id"}, map[string]any{"id": pID})),
	tool("done_notes", "Mark notes done", "Mark notes as done.",
		false, false, schema([]string{"ids"}, map[string]any{"ids": pIDs})),
	tool("reopen_notes", "Reopen notes", "Mark done notes as open again.",
		false, false, schema([]string{"ids"}, map[string]any{"ids": pIDs})),
	tool("delete_notes", "Delete notes", "Permanently delete notes. Prefer done_notes unless the user asked to delete.",
		false, true, schema([]string{"ids"}, map[string]any{"ids": pIDs})),
	tool("remind_note", "Set reminder", `Set or clear the reminder of an existing note. Setting one reopens the note.`,
		false, false, schema([]string{"id", "when"}, map[string]any{
			"id":   pID,
			"when": prop{"type": "string", "description": `a time ("tomorrow 9am", "2h", "fri 14:30") or "off" to clear`},
		})),
	tool("snooze_note", "Snooze reminder", "Remind again after a duration (default 1h).",
		false, false, schema([]string{"id"}, map[string]any{
			"id":       pID,
			"duration": prop{"type": "string", "description": "30m, 2h, 1d (default 1h)"},
		})),
}
