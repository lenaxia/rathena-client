package semantics_test

import (
	"testing"

	"github.com/lenaxia/rathena-client/internal/codegen/semantics"
)

func TestLoadFile_SemanticActions(t *testing.T) {
	db, err := semantics.LoadFile("../../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}

	if len(db.Actions) == 0 {
		t.Fatal("no actions loaded")
	}

	t.Logf("loaded %d actions", len(db.Actions))

	// Check a known simple action: actor_died_or_disappeared.
	a, ok := db.Actions["actor_died_or_disappeared"]
	if !ok {
		t.Fatal("actor_died_or_disappeared not found")
	}
	if a.Description == "" {
		t.Error("description empty")
	}
	if len(a.CanonicalParams) < 2 {
		t.Errorf("expected >=2 canonical params, got %d", len(a.CanonicalParams))
	}
	if len(a.Implementations) == 0 {
		t.Error("no implementations")
	}
	impl := a.Implementations[0]
	if impl.PacketID != "0x0080" {
		t.Errorf("expected packet 0x0080, got %s", impl.PacketID)
	}
	if impl.StructName != "PACKET_ZC_NOTIFY_VANISH" {
		t.Errorf("expected PACKET_ZC_NOTIFY_VANISH, got %s", impl.StructName)
	}
	if len(impl.FieldMapping) == 0 {
		t.Error("field_mapping empty")
	}

	// Check ac_accept_login: multiple implementations.
	aa, ok := db.Actions["ac_accept_login"]
	if !ok {
		t.Fatal("ac_accept_login not found")
	}
	if len(aa.Implementations) == 0 {
		t.Fatal("ac_accept_login: no implementations")
	}
	impl0 := aa.Implementations[0]
	if impl0.PacketID != "0x0069" {
		t.Errorf("ac_accept_login[0]: expected 0x0069, got %s", impl0.PacketID)
	}
	if impl0.PacketverMin == 0 {
		t.Error("ac_accept_login: packetver_min not parsed")
	}
	if len(impl0.FieldMapping) == 0 {
		t.Error("ac_accept_login: field_mapping empty")
	}
	if got := impl0.FieldMapping["AccountID"]; got != "packet.AID" {
		t.Errorf("AccountID mapping: expected 'packet.AID', got %q", got)
	}

	// Check account_id: single implementation.
	ai, ok := db.Actions["account_id"]
	if !ok {
		t.Fatal("account_id not found")
	}
	if len(ai.CanonicalParams) == 0 {
		t.Error("account_id: no canonical params")
	}
	if ai.CanonicalParams[0].Name != "AccountID" {
		t.Errorf("account_id: first param name = %q, want AccountID", ai.CanonicalParams[0].Name)
	}
}
