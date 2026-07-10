// Package mcp implements the MCP (Model Context Protocol) stdio server for
// the semantics-tool command. It exposes the semantic-actions editor
// (internal/semanticsdb) as JSON-RPC tool calls so an MCP-compatible LLM
// client can read and modify semantics/mappings.yaml without touching the
// file directly.
//
// The protocol layer is intentionally minimal: a single-goroutine read loop
// pulls JSON-RPC requests from stdin one per line, dispatches to a tool
// handler, and writes the JSON-RPC response to stdout. There are no
// goroutines spawned by this server — concurrency is the caller's concern,
// matching the rathena-client invariant.
//
// Tools exposed (14 total):
//
//	Read-only:
//	  list_actions, get_action, list_implementations, get_implementation,
//	  search_actions, validate, stats, export
//
//	Mutating:
//	  create_action, update_action, delete_action,
//	  add_implementation, update_implementation, delete_implementation
//
// All mutating tools load → mutate → Save the DB in a single transaction
// (no in-memory persistence between calls). This matches the goKore
// gokore-semantics MCP server's behaviour and makes each call atomic.
package mcp

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/lenaxia/rathena-client/internal/semanticsdb"
)

// MCPRequest / MCPResponse are the JSON-RPC 2.0 envelope types used over
// stdio. The MCP spec ("2024-11-05" at time of writing) layers its own
// semantics on top, but the wire format is plain JSON-RPC.
type MCPRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPTool describes one tool for the tools/list response.
type MCPTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// Server is the MCP stdio server. Create with NewServer, run with Run.
type Server struct {
	mappingsPath string
	out          io.Writer
}

// NewServer returns a Server that edits mappingsPath.
func NewServer(mappingsPath string) *Server {
	return &Server{mappingsPath: mappingsPath, out: os.Stdout}
}

// Run starts the JSON-RPC read loop on stdin. Returns when stdin closes
// (EOF). Returns an error only on stdin read failure (other than EOF) —
// malformed requests and unknown methods are answered with a JSON-RPC error
// and the loop continues.
func (s *Server) Run() error {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		var req MCPRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.sendError(nil, -32700, "Parse error: "+err.Error())
			continue
		}
		s.handleRequest(&req)
	}
}

func (s *Server) handleRequest(req *MCPRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "initialized", "notifications/initialized":
		// No-op notification; MCP clients send this after initialize.
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolCall(req)
	default:
		s.sendError(req.ID, -32601, "Method not found: "+req.Method)
	}
}

func (s *Server) handleInitialize(req *MCPRequest) {
	s.sendResponse(req.ID, map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
		"serverInfo": map[string]string{
			"name":    "rathena-client-semantics",
			"version": "0.1.0",
		},
	})
}

func (s *Server) handleToolsList(req *MCPRequest) {
	s.sendResponse(req.ID, map[string]any{"tools": ToolDefinitions()})
}

func (s *Server) handleToolCall(req *MCPRequest) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params: "+err.Error())
		return
	}

	// Read-only tools share one DB load.
	if isReadonly(params.Name) {
		db, err := semanticsdb.Load(s.mappingsPath)
		if err != nil {
			s.sendError(req.ID, -32000, fmt.Sprintf("Failed to load DB: %v", err))
			return
		}
		result, err := dispatchTool(db, params.Name, params.Arguments)
		if err != nil {
			s.sendError(req.ID, -32000, err.Error())
			return
		}
		s.sendToolResult(req.ID, result)
		return
	}

	// Mutating tools load → mutate → Save in one transaction.
	db, err := semanticsdb.Load(s.mappingsPath)
	if err != nil {
		s.sendError(req.ID, -32000, fmt.Sprintf("Failed to load DB: %v", err))
		return
	}
	result, err := dispatchTool(db, params.Name, params.Arguments)
	if err != nil {
		s.sendError(req.ID, -32000, err.Error())
		return
	}
	if err := db.Save(); err != nil {
		s.sendError(req.ID, -32000, fmt.Sprintf("Save failed: %v", err))
		return
	}
	s.sendToolResult(req.ID, result)
}

func (s *Server) sendToolResult(id interface{}, result any) {
	wrapped := map[string]any{
		"content": []map[string]any{
			{
				"type": "text",
				"text": mustMarshalJSON(result),
			},
		},
	}
	s.sendResponse(id, wrapped)
}

func (s *Server) sendResponse(id interface{}, result interface{}) {
	s.writeJSON(MCPResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) sendError(id interface{}, code int, message string) {
	s.writeJSON(MCPResponse{
		JSONRPC: "2.0", ID: id,
		Error: &MCPError{Code: code, Message: message},
	})
}

func (s *Server) writeJSON(v any) {
	data, _ := json.Marshal(v)
	fmt.Fprintf(s.out, "%s\n", data)
}

func mustMarshalJSON(v any) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}

// isReadonly reports whether the named tool does not mutate the DB.
func isReadonly(name string) bool {
	switch name {
	case "list_actions", "get_action", "list_implementations",
		"get_implementation", "search_actions", "validate", "stats", "export":
		return true
	}
	return false
}
