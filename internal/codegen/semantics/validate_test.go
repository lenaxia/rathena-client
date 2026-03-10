package semantics_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/lenaxia/ragnarok-go-client/internal/codegen/gen"
	"github.com/lenaxia/ragnarok-go-client/internal/codegen/semantics"
)

func TestLoaderDeepValidation(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	t.Logf("Total actions: %d", len(db.Actions))

	type check struct {
		name       string
		wantParams int
		wantImpls  int
		pktID      string
		structName string
		pvMin      int
	}
	checks := []check{
		{"actor_died_or_disappeared", 2, 1, "0x0080", "PACKET_ZC_NOTIFY_VANISH", 20030000},
		{"ac_accept_login", 7, 1, "0x0069", "PACKET_AC_ACCEPT_LOGIN", 20030000},
		{"account_id", 1, 1, "0x0283", "SYNTH_ZC_AID", 20030000},
		{"actor_exists", 35, 4, "0x0078", "packet_idle_unit", 20030000},
		{"move_to", 1, 2, "0x0085", "SYNTH_CZ_REQUEST_MOVE", 20030000},
	}
	for _, c := range checks {
		a, ok := db.Actions[c.name]
		if !ok {
			t.Errorf("MISSING action: %s", c.name)
			continue
		}
		if len(a.CanonicalParams) != c.wantParams {
			t.Errorf("%s: params=%d want=%d", c.name, len(a.CanonicalParams), c.wantParams)
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
		if len(impl0.FieldMapping) == 0 {
			t.Errorf("%s: empty field_mapping", c.name)
		}
	}

	// actor_exists 4th impl (0x09FF) — pvMin bumped to 20181121 when shield field was added
	ae := db.Actions["actor_exists"]
	if ae != nil && len(ae.Implementations) >= 4 {
		impl3 := ae.Implementations[3]
		if impl3.PacketID != "0x09FF" || impl3.StructName != "packet_idle_unit" || impl3.PacketverMin != 20181121 {
			t.Errorf("actor_exists[3]: pkt=%s struct=%s pvMin=%d", impl3.PacketID, impl3.StructName, impl3.PacketverMin)
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

	// ac_accept_login field_mapping
	aal := db.Actions["ac_accept_login"]
	if aal != nil && len(aal.Implementations) > 0 {
		fm := aal.Implementations[0].FieldMapping
		expected := map[string]string{
			"AccountID":     "packet.AID",
			"AccountSex":    "packet.sex",
			"LastLoginIP":   "packet.last_ip",
			"LastLoginTime": "packet.last_login",
			"ServerInfo":    "packet.char_servers",
			"SessionID":     "packet.login_id1",
			"SessionID2":    "packet.login_id2",
		}
		for k, v := range expected {
			if fm[k] != v {
				t.Errorf("ac_accept_login fm[%s]=%q want=%q", k, fm[k], v)
			}
		}
		t.Logf("ac_accept_login field_mapping: %d keys", len(fm))
	}

	// Count coverage
	withImpl, withParam := 0, 0
	for _, a := range db.Actions {
		if len(a.Implementations) > 0 {
			withImpl++
		}
		if len(a.CanonicalParams) > 0 {
			withParam++
		}
	}
	t.Logf("Actions with impls: %d/%d, with params: %d/%d", withImpl, len(db.Actions), withParam, len(db.Actions))

	// Check packetver_max parsing: a null packetver_max should give 0
	for name, a := range db.Actions {
		for _, impl := range a.Implementations {
			if impl.PacketverMax < 0 {
				t.Errorf("%s impl %s: negative packetver_max=%d", name, impl.PacketID, impl.PacketverMax)
			}
		}
	}
	fmt.Printf("") // suppress unused import
}

func TestLoaderImplCounts(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	totalImpls := 0
	totalParams := 0
	multiImpl := 0
	for _, a := range db.Actions {
		totalImpls += len(a.Implementations)
		totalParams += len(a.CanonicalParams)
		if len(a.Implementations) > 1 {
			multiImpl++
		}
	}
	t.Logf("totalImpls=%d totalParams=%d multiImpl=%d", totalImpls, totalParams, multiImpl)
	if totalImpls != 477 {
		t.Errorf("totalImpls=%d want 477", totalImpls)
	}
	if totalParams != 1386 {
		t.Errorf("totalParams=%d want 1386", totalParams)
	}
	if multiImpl != 20 {
		t.Errorf("multiImpl=%d want 20", multiImpl)
	}
}

func TestGeneratedEventsSpotCheck(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	files, err := gen.GenerateEventsDirFiles(db)
	if err != nil {
		t.Fatalf("GenerateEventsDirFiles: %v", err)
	}

	// actor_died_or_disappeared: should have ID uint32 and Type uint8
	src, ok := files["actor_died_or_disappeared.go"]
	if !ok {
		t.Fatal("missing actor_died_or_disappeared.go")
	}
	for _, want := range []string{
		"type ActorDiedOrDisappeared struct",
		"ID uint32",
		"Type uint8",
		"package events",
		"// Code generated",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("actor_died_or_disappeared.go missing %q", want)
		}
	}

	// account_id: should have AccountID uint32
	src, ok = files["account_id.go"]
	if !ok {
		t.Fatal("missing account_id.go")
	}
	if !strings.Contains(src, "AccountID uint32") {
		t.Errorf("account_id.go: missing 'AccountID uint32'\n%s", src)
	}

	// Check invalid type normalisation: no raw pointer types or C-style types in field declarations
	// We check for patterns that look like field lines (tab-indented) with bad types.
	for name, src := range files {
		lines := strings.Split(src, "\n")
		for _, line := range lines {
			// Field declarations look like "\tFieldName TypeName // comment"
			if !strings.HasPrefix(line, "\t") {
				continue
			}
			// Check for raw pointer types (*uint8, *string etc.)
			if strings.Contains(line, " *uint") || strings.Contains(line, " *string") ||
				strings.Contains(line, " *int") {
				t.Errorf("%s: pointer type in field declaration: %q", name, line)
			}
			// Check for C char type
			if strings.Contains(line, "\tchar ") {
				t.Errorf("%s: C char type in field declaration: %q", name, line)
			}
		}
	}

	t.Logf("spot-checked %d event files", len(files))
}
