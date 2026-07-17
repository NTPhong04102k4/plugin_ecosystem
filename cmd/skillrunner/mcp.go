package main

// MCP server mode: `skillrunner serve` exposes the skill pool over the Model
// Context Protocol so any MCP-capable agent (Claude Code, Claude Desktop, ...)
// gets detect/list/emit/apply-base as native tools — no PATH/wrapper juggling.
//
// It speaks JSON-RPC 2.0 over stdio (newline-delimited messages), implemented
// with the standard library only to keep the "single self-contained binary, no
// dependencies" property. stdout carries ONLY protocol messages; all human/log
// output goes to stderr, as the stdio transport requires.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/ghmsoft/skillrunner/internal/skill"
)

// mcpProtocolVersion is the MCP revision this server implements. If the client
// requests a different one we still echo theirs back when possible (the protocol
// tolerates minor-version negotiation for a tools-only server like this).
const mcpProtocolVersion = "2024-11-05"

const mcpServerName = "skillrunner"
const mcpServerVersion = "0.1.0"

// --- JSON-RPC 2.0 envelope types ---

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // absent => notification (no reply)
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// toolArgs is the superset of arguments any of our tools accept; each tool reads
// only the fields it cares about. All are optional except where noted per tool.
type toolArgs struct {
	Dir   string `json:"dir"`
	Pack  string `json:"pack"`
	Skill string `json:"skill"`
	Force bool   `json:"force"`
}

// mcpServer carries the resolved locations of the central skill pool.
type mcpServer struct {
	file    string // manifest path (skill.json)
	packDir string // directory holding packs/
	baseDir string // default project dir when a tool call omits "dir"
	out     *json.Encoder
}

// runMCPServer reads JSON-RPC requests from stdin and serves them until EOF.
func runMCPServer(file, baseDir, packDir string) error {
	srv := &mcpServer{
		file:    file,
		packDir: packDir,
		baseDir: baseDir,
		out:     json.NewEncoder(os.Stdout),
	}
	fmt.Fprintf(os.Stderr, "skillrunner MCP server on stdio (manifest=%s, packs=%s/packs)\n", file, packDir)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			// Can't recover an ID from an unparseable message; skip it.
			fmt.Fprintf(os.Stderr, "note: dropping unparseable message (%v)\n", err)
			continue
		}
		srv.dispatch(&req)
	}
	return scanner.Err()
}

// dispatch routes one request. Notifications (no ID) never get a reply.
func (s *mcpServer) dispatch(req *rpcRequest) {
	isNotification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		s.reply(req.ID, s.initializeResult(req.Params))
	case "notifications/initialized":
		// Handshake ack — nothing to do, no reply.
	case "ping":
		s.reply(req.ID, map[string]any{})
	case "tools/list":
		s.reply(req.ID, map[string]any{"tools": toolDefs()})
	case "tools/call":
		s.handleToolCall(req)
	default:
		if !isNotification {
			s.replyError(req.ID, -32601, "method not found: "+req.Method)
		}
	}
}

// initializeResult echoes the client's protocol version when given, and declares
// the tools capability.
func (s *mcpServer) initializeResult(params json.RawMessage) map[string]any {
	version := mcpProtocolVersion
	if len(params) > 0 {
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(params, &p); err == nil && p.ProtocolVersion != "" {
			version = p.ProtocolVersion
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": mcpServerName, "version": mcpServerVersion},
	}
}

// --- tool definitions ---

// toolDefs returns the tool schemas advertised via tools/list.
func toolDefs() []map[string]any {
	dirProp := map[string]any{"type": "string", "description": "Project directory to act on (default: the server's working dir)."}
	packProp := map[string]any{"type": "string", "description": "Force a stack pack (e.g. react, go). Default: auto-detect."}
	return []map[string]any{
		{
			"name":        "detect_stack",
			"description": "Detect the project's stack (pack) by inspecting its files.",
			"inputSchema": objectSchema(map[string]any{"dir": dirProp}, nil),
		},
		{
			"name":        "list_skills",
			"description": "List available skills (auto-detects & merges the stack pack).",
			"inputSchema": objectSchema(map[string]any{"dir": dirProp, "pack": packProp}, nil),
		},
		{
			"name":        "emit_skill",
			"description": "Print the marching orders for a skill (or \"all\" for the full catalog). Records a real skill in the project ledger.",
			"inputSchema": objectSchema(map[string]any{
				"skill": map[string]any{"type": "string", "description": "Skill name, or \"all\" for every skill."},
				"dir":   dirProp,
				"pack":  packProp,
			}, []string{"skill"}),
		},
		{
			"name":        "apply_base",
			"description": "Copy the stack's base config files (eslint/linter/tsconfig/...) into the project. Skips existing files unless force is true.",
			"inputSchema": objectSchema(map[string]any{
				"dir":   dirProp,
				"pack":  packProp,
				"force": map[string]any{"type": "boolean", "description": "Overwrite existing files instead of skipping them."},
			}, nil),
		},
		{
			"name":        "status",
			"description": "Show the detected stack and whether the project profile/registry/ledger are cached.",
			"inputSchema": objectSchema(map[string]any{"dir": dirProp}, nil),
		},
	}
}

// objectSchema builds a JSON Schema object node with the given properties and
// required list.
func objectSchema(props map[string]any, required []string) map[string]any {
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// --- tool call handling ---

func (s *mcpServer) handleToolCall(req *rpcRequest) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &call); err != nil {
		s.replyError(req.ID, -32602, "invalid params: "+err.Error())
		return
	}
	var args toolArgs
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			s.replyToolError(req.ID, "invalid arguments: "+err.Error())
			return
		}
	}
	dir := args.Dir
	if dir == "" {
		dir = s.baseDir
	}

	text, err := s.runTool(call.Name, dir, args)
	if err != nil {
		s.replyToolError(req.ID, err.Error())
		return
	}
	s.replyToolText(req.ID, text)
}

// runTool executes one tool and returns its text output. Reuses the same
// package-level logic the CLI commands use, so behavior stays identical.
func (s *mcpServer) runTool(name, dir string, args toolArgs) (string, error) {
	switch name {
	case "detect_stack":
		d := skill.Detect(dir)
		if d.Stack == "" {
			return fmt.Sprintf("No known stack detected (%s)", d.Reason), nil
		}
		return fmt.Sprintf("Detected stack: %s (%s)", d.Stack, d.Reason), nil

	case "list_skills":
		m, err := loadWithPack(s.file, dir, s.packDir, args.Pack)
		if err != nil {
			return "", err
		}
		return m.List(), nil

	case "emit_skill":
		if args.Skill == "" {
			return "", fmt.Errorf("emit_skill requires a \"skill\" argument (or \"all\")")
		}
		m, err := loadWithPack(s.file, dir, s.packDir, args.Pack)
		if err != nil {
			return "", err
		}
		if args.Skill == "all" {
			return m.EmitAll()
		}
		out, err := m.Emit(args.Skill)
		if err != nil {
			return "", err
		}
		// Record the emit in the project ledger, mirroring the CLI. A ledger
		// failure must not lose the orders we already produced — note and carry on.
		stack := args.Pack
		if stack == "" {
			stack = skill.Detect(dir).Stack
		}
		if err := skill.RecordEmit(dir, projectLabel(dir), args.Skill, stack, time.Now()); err != nil {
			fmt.Fprintf(os.Stderr, "note: could not record emit in ledger (%v)\n", err)
		}
		return out, nil

	case "apply_base":
		stack := args.Pack
		if stack == "" {
			stack = skill.Detect(dir).Stack
		}
		if stack == "" {
			return "", fmt.Errorf("could not detect a stack; pass \"pack\" (available: %v)", skill.AvailablePacks(s.packDir))
		}
		p, err := skill.LoadPack(s.packDir, stack)
		if err != nil {
			return "", err
		}
		results, err := p.ApplyBase(s.packDir, dir, args.Force)
		if err != nil {
			return "", err
		}
		return formatApplyResults(stack, dir, results, args.Force), nil

	case "status":
		return s.statusText(dir), nil

	default:
		return "", fmt.Errorf("unknown tool %q", name)
	}
}

// formatApplyResults renders apply-base output as text for a tool response.
func formatApplyResults(stack, dir string, results []skill.ApplyResult, force bool) string {
	var b []byte
	b = fmt.Appendf(b, "apply-base [%s] -> %s\n", stack, dir)
	for _, r := range results {
		b = fmt.Appendf(b, "  %s %-28s %s\n", applyMark(r.Status), r.To, applyNote(r))
	}
	if !force && anySkipped(results) {
		b = fmt.Appendf(b, "\nSome files already existed and were skipped. Call again with force=true to overwrite.")
	}
	return string(b)
}

// statusText mirrors the CLI `status` command as a single string.
func (s *mcpServer) statusText(dir string) string {
	var b []byte
	d := skill.Detect(dir)
	if d.Stack == "" {
		b = fmt.Appendf(b, "Stack:   unknown (%s)\n", d.Reason)
	} else {
		b = fmt.Appendf(b, "Stack:   %s (%s)\n", d.Stack, d.Reason)
	}
	b = append(b, cacheLine(dir, "Profile", "docs/project-profile.md", "run learn-project to build it")...)
	b = append(b, cacheLine(dir, "Registry", "docs/module-registry.md", "will be created as features land")...)
	if l, err := skill.LoadLedger(dir); err == nil {
		b = fmt.Appendf(b, "%s\n", l.StatusLine())
	}
	return string(b)
}

// --- reply helpers ---

func (s *mcpServer) reply(id json.RawMessage, result any) {
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *mcpServer) replyError(id json.RawMessage, code int, msg string) {
	s.write(rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg}})
}

// replyToolText / replyToolError return a tools/call result. Tool-level failures
// are reported as isError content (not JSON-RPC errors), per the MCP spec.
func (s *mcpServer) replyToolText(id json.RawMessage, text string) {
	s.reply(id, map[string]any{"content": []map[string]any{{"type": "text", "text": text}}})
}

func (s *mcpServer) replyToolError(id json.RawMessage, text string) {
	s.reply(id, map[string]any{
		"isError": true,
		"content": []map[string]any{{"type": "text", "text": text}},
	})
}

func (s *mcpServer) write(resp rpcResponse) {
	if err := s.out.Encode(resp); err != nil {
		fmt.Fprintf(os.Stderr, "note: failed to write response (%v)\n", err)
	}
}
