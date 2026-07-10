package cli_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lenaxia/rathena-client/cmd/semantics-tool/cli"
)

// sampleYAML mirrors the structure of the real mappings.yaml but at a tiny
// scale. Two existing actions plus the surrounding sections.
const sampleYAML = `mappings:
    - packet_id: "0x0103"
      direction: send
      rathena_struct: SYNTH_CZ_REQ_EXPEL_GROUP_MEMBER
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

func runCLI(t *testing.T, mappingsPath string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	fullArgs := append([]string{"--file", mappingsPath}, args...)
	err := cli.Run(fullArgs, &out)
	return out.String(), err
}

func TestCLI_Stats(t *testing.T) {
	p := writeSample(t)
	out, err := runCLI(t, p, "stats")
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	var s map[string]int
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		t.Fatalf("unmarshal: %v\noutput: %s", err, out)
	}
	if s["ActionCount"] != 1 {
		t.Errorf("ActionCount = %d, want 1", s["ActionCount"])
	}
}

func TestCLI_ListActions(t *testing.T) {
	p := writeSample(t)
	out, err := runCLI(t, p, "list-actions")
	if err != nil {
		t.Fatalf("list-actions: %v", err)
	}
	if !strings.Contains(out, "actor_died_or_disappeared") {
		t.Errorf("expected action name in output, got: %s", out)
	}
}

func TestCLI_GetAction(t *testing.T) {
	p := writeSample(t)
	out, err := runCLI(t, p, "get-action", "actor_died_or_disappeared")
	if err != nil {
		t.Fatalf("get-action: %v", err)
	}
	if !strings.Contains(out, "0x0080") {
		t.Errorf("expected packet_id in output, got: %s", out)
	}
}

func TestCLI_Search(t *testing.T) {
	p := writeSample(t)
	out, err := runCLI(t, p, "search", "-struct", "VANISH")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !strings.Contains(out, "actor_died_or_disappeared") {
		t.Errorf("expected match in output, got: %s", out)
	}
}

func TestCLI_CreateAction_FlagsBeforePositional(t *testing.T) {
	p := writeSample(t)
	out, err := runCLI(t, p, "create-action", "-description", "test desc", "-openkore", "openk", "zc_new_one")
	if err != nil {
		t.Fatalf("create-action: %v\noutput: %s", err, out)
	}

	// Reload and verify the action persisted.
	out2, err := runCLI(t, p, "get-action", "zc_new_one")
	if err != nil {
		t.Fatalf("get-action after create: %v", err)
	}
	if !strings.Contains(out2, "test desc") {
		t.Errorf("description not persisted: %s", out2)
	}
	if !strings.Contains(out2, "openk") {
		t.Errorf("openkore_name not persisted: %s", out2)
	}
}

func TestCLI_AddImplementation_FlowStyleFix(t *testing.T) {
	// Regression: appending implementations to a freshly-created action
	// (which started with empty `implementations: []`) must produce BLOCK
	// style in the output, matching the rest of mappings.yaml. The flow
	// style bug was: `implementations: [{packet_id: ...}]` (one line).
	p := writeSample(t)
	if _, err := runCLI(t, p, "create-action", "zc_new_one"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCLI(t, p, "add-implementation", "-id", "0x00FB", "-struct", "PACKET_X", "zc_new_one"); err != nil {
		t.Fatal(err)
	}

	// Read the file directly and check format.
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	str := string(data)

	// Must contain block-style implementations list.
	if !strings.Contains(str, "implementations:\n") {
		t.Errorf("expected block-style implementations list, file content:\n%s", str)
	}
	if !strings.Contains(str, "packet_id: \"0x00FB\"") {
		t.Errorf("expected packet_id 0x00FB in output, got:\n%s", str)
	}
	// Must NOT contain flow-style list.
	if strings.Contains(str, "implementations: [{") {
		t.Errorf("REGRESSION: flow-style implementations list detected:\n%s", str)
	}
}

func TestCLI_AddThreeImplementations_MultiVariantScenario(t *testing.T) {
	// End-to-end test for the multi-variant workflow: create an action with
	// three packetver-bounded implementations (mirrors what real-world
	// actions like zc_notify_mapproperty2 need).
	p := writeSample(t)

	if _, err := runCLI(t, p,
		"create-action",
		"-description", "Multi-variant test action",
		"-openkore", "test_action",
		"zc_test_multi"); err != nil {
		t.Fatalf("create-action: %v", err)
	}

	for _, c := range []struct {
		id, structName string
		min, max       int
	}{
		{"0x00FB", "PACKET_TEST_STRUCT", 0, 20170501},
		{"0x0A44", "PACKET_TEST_STRUCT", 20170502, 20171206},
		{"0x0AE5", "PACKET_TEST_STRUCT", 20171207, 0},
	} {
		args := []string{"add-implementation", "-id", c.id, "-struct", c.structName, "zc_test_multi"}
		if c.min != 0 {
			args = append(args, "-min", fmtInt(c.min))
		}
		if c.max != 0 {
			args = append(args, "-max", fmtInt(c.max))
		}
		if _, err := runCLI(t, p, args...); err != nil {
			t.Fatalf("add-implementation %s: %v", c.id, err)
		}
	}

	// Validate clean.
	if _, err := runCLI(t, p, "validate"); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Reload and verify the action has three implementations.
	out, err := runCLI(t, p, "get-action", "zc_test_multi")
	if err != nil {
		t.Fatalf("get-action: %v", err)
	}
	count := strings.Count(out, "PACKET_TEST_STRUCT")
	if count != 3 {
		t.Errorf("expected 3 struct_name occurrences, got %d in:\n%s", count, out)
	}
}

func TestCLI_DeleteAction(t *testing.T) {
	p := writeSample(t)
	if _, err := runCLI(t, p, "delete-action", "actor_died_or_disappeared"); err != nil {
		t.Fatalf("delete-action: %v", err)
	}
	if _, err := runCLI(t, p, "get-action", "actor_died_or_disappeared"); err == nil {
		t.Error("expected error getting deleted action")
	}
}

func TestCLI_DeleteImplementation(t *testing.T) {
	p := writeSample(t)
	if _, err := runCLI(t, p, "delete-implementation", "-id", "0x0080", "actor_died_or_disappeared"); err != nil {
		t.Fatalf("delete-implementation: %v", err)
	}
	// Action should still exist but have zero impls.
	out, err := runCLI(t, p, "get-action", "actor_died_or_disappeared")
	if err != nil {
		t.Fatalf("get-action: %v", err)
	}
	if strings.Contains(out, "0x0080") {
		t.Errorf("impl still present after delete: %s", out)
	}
}

func TestCLI_UnknownCommand(t *testing.T) {
	p := writeSample(t)
	_, err := runCLI(t, p, "bogus-command")
	if err == nil {
		t.Error("expected error on unknown command")
	}
}

func TestCLI_Help(t *testing.T) {
	p := writeSample(t)
	out, err := runCLI(t, p, "help")
	if err != nil {
		t.Fatalf("help: %v", err)
	}
	if !strings.Contains(out, "list-actions") {
		t.Errorf("help output missing command list: %s", out)
	}
}

func TestCLI_PreservesUnrelatedSections(t *testing.T) {
	p := writeSample(t)
	if _, err := runCLI(t, p, "create-action", "zc_new_one"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	str := string(data)

	// Flat mappings: section must survive.
	if !strings.Contains(str, "rathena_struct: SYNTH_CZ_REQ_EXPEL_GROUP_MEMBER") {
		t.Error("flat mappings: section dropped")
	}
	// metadata: section must survive.
	if !strings.Contains(str, "metadata:") || !strings.Contains(str, "total_packets: 1") {
		t.Error("metadata: section dropped")
	}
	// Trailing version: line must survive.
	if !strings.Contains(str, `version: ""`) {
		t.Error("trailing version: line dropped")
	}
}

func fmtInt(n int) string {
	if n == 0 {
		return "0"
	}
	var sign string
	if n < 0 {
		sign = "-"
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return sign + string(digits)
}
