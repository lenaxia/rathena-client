package semantics_test

import (
	"strings"
	"testing"

	"github.com/lenaxia/rathena-client/internal/codegen/gen"
	"github.com/lenaxia/rathena-client/internal/codegen/preprocess"
	"github.com/lenaxia/rathena-client/internal/codegen/semantics"
)

func TestLoaderDeepValidation(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	t.Logf("Total actions: %d", len(db.Actions))

	type check struct {
		name       string
		wantImpls  int
		pktID      string
		structName string
		pvMin      int
	}
	checks := []check{
		{"actor_died_or_disappeared", 1, "0x0080", "PACKET_ZC_NOTIFY_VANISH", 20030000},
		{"ac_accept_login", 2, "0x0069", "PACKET_AC_ACCEPT_LOGIN", 20030000},
		{"account_id", 1, "0x0283", "SYNTH_ZC_AID", 20030000},
		{"actor_exists", 9, "0x0078", "packet_idle_unit", 20030000},
		{"move_to", 2, "0x0085", "SYNTH_CZ_REQUEST_MOVE", 20030000},
	}
	for _, c := range checks {
		a, ok := db.Actions[c.name]
		if !ok {
			t.Errorf("MISSING action: %s", c.name)
			continue
		}
		if len(a.Implementations) != c.wantImpls {
			t.Errorf("%s: impls=%d want=%d", c.name, len(a.Implementations), c.wantImpls)
			continue
		}
		impl0 := a.Implementations[0]
		if impl0.PacketID != c.pktID {
			t.Errorf("%s: pktID=%q want=%q", c.name, impl0.PacketID, c.pktID)
		}
		if impl0.StructName != c.structName {
			t.Errorf("%s: struct=%q want=%q", c.name, impl0.StructName, c.structName)
		}
		if impl0.PacketverMin != c.pvMin {
			t.Errorf("%s: pvMin=%d want=%d", c.name, impl0.PacketverMin, c.pvMin)
		}
	}

	// actor_exists must contain 0x09FF with pvMin=20181121
	ae := db.Actions["actor_exists"]
	if ae != nil {
		found := false
		for _, impl := range ae.Implementations {
			if impl.PacketID == "0x09FF" && impl.StructName == "packet_idle_unit" && impl.PacketverMin == 20181121 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("actor_exists: missing 0x09FF/packet_idle_unit/pvMin=20181121")
		}
	}

	// move_to 2nd impl (0x0437)
	mt := db.Actions["move_to"]
	if mt != nil && len(mt.Implementations) >= 2 {
		impl1 := mt.Implementations[1]
		if impl1.PacketID != "0x0437" || impl1.StructName != "SYNTH_CZ_REQUEST_MOVE2" {
			t.Errorf("move_to[1]: pkt=%s struct=%s", impl1.PacketID, impl1.StructName)
		}
	}

	// Count coverage
	withImpl := 0
	for _, a := range db.Actions {
		if len(a.Implementations) > 0 {
			withImpl++
		}
	}
	t.Logf("Actions with impls: %d/%d", withImpl, len(db.Actions))

	// Check packetver_max parsing: a null packetver_max should give 0
	for name, a := range db.Actions {
		for _, impl := range a.Implementations {
			if impl.PacketverMax < 0 {
				t.Errorf("%s impl %s: negative packetver_max=%d", name, impl.PacketID, impl.PacketverMax)
			}
		}
	}
}

func TestLoaderImplCounts(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	totalImpls := 0
	multiImpl := 0
	for _, a := range db.Actions {
		totalImpls += len(a.Implementations)
		if len(a.Implementations) > 1 {
			multiImpl++
		}
	}
	t.Logf("totalImpls=%d multiImpl=%d actions=%d", totalImpls, multiImpl, len(db.Actions))
	// Sanity bounds — exact count may vary as DB evolves.
	if totalImpls < 400 {
		t.Errorf("totalImpls=%d looks too low (want >= 400)", totalImpls)
	}
	if len(db.Actions) < 200 {
		t.Errorf("actions=%d looks too low (want >= 200)", len(db.Actions))
	}
}

func TestGeneratedEventsSpotCheck(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	// Use an empty VersionTable — generates empty event structs, still valid Go.
	vt := make(preprocess.VersionTable)
	files, err := gen.GenerateEventsDirFiles(db, vt)
	if err != nil {
		t.Fatalf("GenerateEventsDirFiles: %v", err)
	}

	// actor_died_or_disappeared: struct must be present (fields populated by codegen run)
	src, ok := files["actor_died_or_disappeared.go"]
	if !ok {
		t.Fatal("missing actor_died_or_disappeared.go")
	}
	for _, want := range []string{
		"type ActorDiedOrDisappeared struct",
		"package events",
		"// Code generated",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("actor_died_or_disappeared.go missing %q", want)
		}
	}

	// Check no raw pointer types or C-style types in field declarations.
	for name, fsrc := range files {
		lines := strings.Split(fsrc, "\n")
		for _, line := range lines {
			if !strings.HasPrefix(line, "\t") {
				continue
			}
			if strings.Contains(line, " *uint") || strings.Contains(line, " *string") ||
				strings.Contains(line, " *int") {
				t.Errorf("%s: pointer type in field declaration: %q", name, line)
			}
			if strings.Contains(line, "\tchar ") {
				t.Errorf("%s: C char type in field declaration: %q", name, line)
			}
		}
	}

	t.Logf("spot-checked %d event files", len(files))
}
