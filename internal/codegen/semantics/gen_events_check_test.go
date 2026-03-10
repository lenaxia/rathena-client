package semantics_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/lenaxia/rathena-client/internal/codegen/gen"
	"github.com/lenaxia/rathena-client/internal/codegen/preprocess"
	"github.com/lenaxia/rathena-client/internal/codegen/semantics"
)

func TestGeneratedEventsValidGo(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	// Use an empty VersionTable — event files are still generated (empty structs).
	vt := make(preprocess.VersionTable)
	files, err := gen.GenerateEventsDirFiles(db, vt)
	if err != nil {
		t.Fatalf("GenerateEventsDirFiles: %v", err)
	}

	tmpDir := t.TempDir()
	fset := token.NewFileSet()

	parseErrors := 0
	for filename, src := range files {
		path := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", filename, err)
		}
		_, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if err != nil {
			t.Errorf("parse error in %s: %v", filename, err)
			parseErrors++
			if parseErrors > 5 {
				t.Log("... (too many errors, truncating)")
				break
			}
		}
	}
	t.Logf("parsed %d event files, %d parse errors", len(files), parseErrors)
}
