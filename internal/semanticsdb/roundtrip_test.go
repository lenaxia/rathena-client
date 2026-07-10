package semanticsdb_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lenaxia/rathena-client/internal/semanticsdb"
)

// TestProductionMappings_RoundTripByteIdentical loads the real production
// mappings.yaml, immediately saves it without any mutation, and verifies the
// output is byte-identical to the input. This is the strongest guarantee we
// can offer consumers: "no-op Save does not modify the file".
//
// If this test fails, Save is leaking formatter drift (e.g. yaml.v3 reordered
// keys, changed quote style, dropped a comment). Fix before shipping.
func TestProductionMappings_RoundTripByteIdentical(t *testing.T) {
	orig, err := os.ReadFile("../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("read production mappings.yaml: %v", err)
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(p, orig, 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	db, err := semanticsdb.Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := db.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	after, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read after save: %v", err)
	}

	if string(orig) != string(after) {
		t.Errorf("round-trip changed %d-byte file into %d bytes", len(orig), len(after))

		minLen := len(orig)
		if len(after) < minLen {
			minLen = len(after)
		}
		for i := 0; i < minLen; i++ {
			if orig[i] != after[i] {
				start := i - 80
				if start < 0 {
					start = 0
				}
				end := i + 80
				if end > minLen {
					end = minLen
				}
				t.Errorf("first diff at byte %d\norig: %s\nafter: %s",
					i,
					strings.ReplaceAll(string(orig[start:end]), "\n", "\\n"),
					strings.ReplaceAll(string(after[start:end]), "\n", "\\n"))
				break
			}
		}
		// If no per-char diff was found, lengths differ.
		if len(orig) != len(after) {
			t.Errorf("no per-char diff found but lengths differ: %d vs %d", len(orig), len(after))
		}
	}
}
