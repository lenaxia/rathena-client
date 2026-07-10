package mcp_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const sampleYAML = `mappings:
    - packet_id: "0x0103"
metadata:
    total_packets: 1
semantic_actions:
    actor_died_or_disappeared:
        name: ""
        description: ""
        openkore_name: actor_muted
        canonical_params: []
        implementations:
            - packet_id: "0x0080"
              packetver_range:
                - null
                - null
              struct_name: PACKET_ZC_NOTIFY_VANISH
              field_mapping: {}
version: ""
`

func writeSample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(p, []byte(sampleYAML), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	return p
}

// cachedBinary is the path of a built semantics-tool binary, computed once
// per test process. Building once and reusing saves ~5s of `go build` per
// test.
var (
	cachedBinaryOnce sync.Once
	cachedBinary     string
	cachedBinaryErr  error
)

func buildBinary(t *testing.T) string {
	t.Helper()
	cachedBinaryOnce.Do(func() {
		dir, err := os.MkdirTemp("", "semtool-test")
		if err != nil {
			cachedBinaryErr = err
			return
		}
		bin := filepath.Join(dir, "semantics-tool")
		// Resolve the module root from the test file's location:
		// this file lives at <module>/cmd/semantics-tool/mcp/server_test.go
		// so the module root is three ".." up.
		wd, err := os.Getwd()
		if err != nil {
			cachedBinaryErr = err
			return
		}
		moduleRoot := filepath.Join(wd, "..", "..", "..")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/semantics-tool")
		cmd.Dir = moduleRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			cachedBinaryErr = fmt.Errorf("go build (dir=%s): %v\n%s", moduleRoot, err, out)
			return
		}
		cachedBinary = bin
	})
	if cachedBinaryErr != nil {
		t.Fatalf("build: %v", cachedBinaryErr)
	}
	return cachedBinary
}

// runSession runs the MCP server as a subprocess, sends the requests in
// order, and returns the JSON-RPC responses (skipping notifications which
// have no `id`).
func runSession(t *testing.T, mappingsPath string, requests []map[string]any) []map[string]any {
	t.Helper()
	bin := buildBinary(t)

	cmd := exec.Command(bin, "serve", "--file", mappingsPath)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	defer func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	}()

	// Send all requests up front.
	go func() {
		defer stdin.Close()
		for _, req := range requests {
			b, _ := json.Marshal(req)
			stdin.Write(append(b, '\n'))
		}
	}()

	var responses []map[string]any
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for len(responses) < len(requests) {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				t.Fatalf("scanner: %v", err)
			}
			t.Fatalf("server closed stdout before %d responses arrived (got %d)",
				len(requests), len(responses))
		}
		line := scanner.Bytes()
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		var resp map[string]any
		if err := json.Unmarshal(line, &resp); err != nil {
			t.Fatalf("unmarshal response %q: %v", line, err)
		}
		// Skip notifications (no id).
		if _, hasID := resp["id"]; !hasID {
			continue
		}
		responses = append(responses, resp)
	}
	return responses
}

func req(id any, method string, params any) map[string]any {
	return map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
}

func TestMCP_Initialize(t *testing.T) {
	p := writeSample(t)
	resps := runSession(t, p, []map[string]any{
		req(1, "initialize", map[string]any{}),
	})
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if resps[0]["error"] != nil {
		t.Fatalf("initialize returned error: %v", resps[0]["error"])
	}
	result := resps[0]["result"].(map[string]any)
	if result["protocolVersion"] == nil {
		t.Error("no protocolVersion in initialize result")
	}
}

func TestMCP_ToolsList_ReturnsAll14Tools(t *testing.T) {
	p := writeSample(t)
	resps := runSession(t, p, []map[string]any{
		req(1, "initialize", map[string]any{}),
		req(2, "tools/list", map[string]any{}),
	})
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
	result := resps[1]["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 14 {
		t.Errorf("expected 14 tools, got %d", len(tools))
	}
}

func TestMCP_Stats(t *testing.T) {
	p := writeSample(t)
	resps := runSession(t, p, []map[string]any{
		req(1, "initialize", map[string]any{}),
		req(2, "tools/call", map[string]any{
			"name":      "stats",
			"arguments": map[string]any{},
		}),
	})
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
	if resps[1]["error"] != nil {
		t.Fatalf("stats error: %v", resps[1]["error"])
	}
}

func TestMCP_CreateAction_AndListActions(t *testing.T) {
	p := writeSample(t)
	resps := runSession(t, p, []map[string]any{
		req(1, "initialize", map[string]any{}),
		req(2, "tools/call", map[string]any{
			"name": "create_action",
			"arguments": map[string]any{
				"action_name":   "zc_group_list",
				"description":   "Party roster",
				"openkore_name": "party_users",
			},
		}),
		req(3, "tools/call", map[string]any{
			"name":      "list_actions",
			"arguments": map[string]any{},
		}),
	})
	if len(resps) != 3 {
		t.Fatalf("expected 3 responses, got %d", len(resps))
	}
	if resps[1]["error"] != nil {
		t.Fatalf("create_action error: %v", resps[1]["error"])
	}
	// list_actions response should now contain zc_group_list.
	result := resps[2]["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "zc_group_list") {
		t.Errorf("list_actions missing zc_group_list: %s", text)
	}
}

func TestMCP_AddImplementation(t *testing.T) {
	p := writeSample(t)
	resps := runSession(t, p, []map[string]any{
		req(1, "initialize", map[string]any{}),
		req(2, "tools/call", map[string]any{
			"name": "create_action",
			"arguments": map[string]any{
				"action_name": "zc_group_list",
			},
		}),
		req(3, "tools/call", map[string]any{
			"name": "add_implementation",
			"arguments": map[string]any{
				"action_name":   "zc_group_list",
				"packet_id":     "0x00FB",
				"struct_name":   "PACKET_ZC_GROUP_LIST",
				"packetver_max": 20170501,
			},
		}),
		req(4, "tools/call", map[string]any{
			"name": "get_action",
			"arguments": map[string]any{
				"action_name": "zc_group_list",
			},
		}),
	})
	if len(resps) != 4 {
		t.Fatalf("expected 4 responses, got %d", len(resps))
	}
	// get_action response should contain the implementation.
	result := resps[3]["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "0x00FB") {
		t.Errorf("get_action missing impl: %s", text)
	}
	if !strings.Contains(text, "PACKET_ZC_GROUP_LIST") {
		t.Errorf("get_action missing struct: %s", text)
	}
}

func TestMCP_Validate(t *testing.T) {
	p := writeSample(t)
	resps := runSession(t, p, []map[string]any{
		req(1, "initialize", map[string]any{}),
		req(2, "tools/call", map[string]any{
			"name":      "validate",
			"arguments": map[string]any{},
		}),
	})
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
	if resps[1]["error"] != nil {
		t.Fatalf("validate error: %v", resps[1]["error"])
	}
	result := resps[1]["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "valid") {
		t.Errorf("validate response missing 'valid' field: %s", text)
	}
}

func TestMCP_ErrorOnUnknownTool(t *testing.T) {
	p := writeSample(t)
	resps := runSession(t, p, []map[string]any{
		req(1, "initialize", map[string]any{}),
		req(2, "tools/call", map[string]any{
			"name":      "bogus_tool",
			"arguments": map[string]any{},
		}),
	})
	if len(resps) != 2 {
		t.Fatalf("expected 2 responses, got %d", len(resps))
	}
	if resps[1]["error"] == nil {
		t.Error("expected error on unknown tool")
	}
}
