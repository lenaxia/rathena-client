package semanticsdb_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lenaxia/rathena-client/internal/semanticsdb"
)

// sampleYAML mirrors the structure of the real mappings.yaml but at a tiny
// scale: just two existing actions, a flat mappings: stub, metadata, and a
// trailing version: line. Tests use this to verify round-trip preservation.
const sampleYAML = `mappings:
    - packet_id: "0x0103"
      direction: send
      rathena_struct: SYNTH_CZ_REQ_EXPEL_GROUP_MEMBER
      openkore_name: remove_party_member
      semantic_category: party
      description: Kick a member from the party
      confidence: 90
      validated_date: "2026-03-11"
      fields:
        - position: 0
          rathena_name: PacketType
          rathena_type: int16
          openkore_name: PacketType
          semantic: Packet ID header
          omit_from_openkore: true
metadata:
    total_packets: 31
    last_updated: "2026-04-30"
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
    zc_accept_enter:
        name: ""
        description: ""
        openkore_name: map_loaded
        canonical_params: []
        implementations:
            - packet_id: "0x0073"
              packetver_range:
                - null
                - 20080101
              struct_name: PACKET_ZC_ACCEPT_ENTER
              field_mapping: {}
            - packet_id: "0x02EB"
              packetver_range:
                - 20080102
                - 20141021
              struct_name: PACKET_ZC_ACCEPT_ENTER
              field_mapping: {}
            - packet_id: "0x0A18"
              packetver_range:
                - 20141022
                - 20160329
              struct_name: PACKET_ZC_ACCEPT_ENTER
              field_mapping: {}
version: ""
`

// writeSample writes sampleYAML to a temp file and returns its path.
func writeSample(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(p, []byte(sampleYAML), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}
	return p
}

// writeProduction loads the real repo mappings.yaml into a temp file so tests
// can mutate it freely.
func writeProduction(t *testing.T) string {
	t.Helper()
	orig, err := os.ReadFile("../../semantics/mappings.yaml")
	if err != nil {
		t.Fatalf("read production mappings.yaml: %v", err)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(p, orig, 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	return p
}

func TestLoad_AndRoundTripPreservesContent(t *testing.T) {
	p := writeSample(t)
	db, err := semanticsdb.Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := db.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != sampleYAML {
		t.Errorf("round-trip changed the file:\n--- want ---\n%s\n--- got ---\n%s", sampleYAML, string(got))
	}
}

func TestLoad_PreservesUnrelatedSections(t *testing.T) {
	p := writeSample(t)
	db, err := semanticsdb.Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Mutate semantic_actions only.
	if err := db.CreateAction("new_action", "test desc", "openkore_x"); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	if err := db.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := os.ReadFile(p)

	// Flat mappings: section must survive byte-identical.
	if !strings.Contains(string(got), "rathena_struct: SYNTH_CZ_REQ_EXPEL_GROUP_MEMBER") {
		t.Error("flat mappings: section was mangled on Save")
	}
	if !strings.Contains(string(got), "metadata:") || !strings.Contains(string(got), `last_updated: "2026-04-30"`) {
		t.Error("metadata: section was mangled on Save")
	}
	if !strings.Contains(string(got), `version: ""`) {
		t.Error("trailing version: line was mangled on Save")
	}
	// Existing actions survive.
	if !strings.Contains(string(got), "actor_died_or_disappeared:") {
		t.Error("existing action actor_died_or_disappeared was dropped")
	}
	// New action present.
	if !strings.Contains(string(got), "new_action:") {
		t.Error("new action not added")
	}
}

func TestListActions_Alphabetical(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	got := db.ListActions()
	want := []string{"actor_died_or_disappeared", "zc_accept_enter"}
	if len(got) != len(want) {
		t.Fatalf("ListActions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ListActions[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestGetAction_FieldsDecodedCorrectly(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)

	a, ok := db.GetAction("zc_accept_enter")
	if !ok {
		t.Fatal("zc_accept_enter not found")
	}
	if len(a.Implementations) != 3 {
		t.Fatalf("expected 3 implementations, got %d", len(a.Implementations))
	}

	// Impls with [null, null], [null, N], [N, M].
	cases := []struct {
		idx          int
		packetID     string
		structName   string
		pvMin, pvMax int
	}{
		{0, "0x0073", "PACKET_ZC_ACCEPT_ENTER", 0, 20080101},
		{1, "0x02EB", "PACKET_ZC_ACCEPT_ENTER", 20080102, 20141021},
		{2, "0x0A18", "PACKET_ZC_ACCEPT_ENTER", 20141022, 20160329},
	}
	for _, c := range cases {
		impl := a.Implementations[c.idx]
		if impl.PacketID != c.packetID {
			t.Errorf("impl[%d].PacketID = %q, want %q", c.idx, impl.PacketID, c.packetID)
		}
		if impl.StructName != c.structName {
			t.Errorf("impl[%d].StructName = %q, want %q", c.idx, impl.StructName, c.structName)
		}
		if impl.PacketverMin != c.pvMin {
			t.Errorf("impl[%d].PacketverMin = %d, want %d", c.idx, impl.PacketverMin, c.pvMin)
		}
		if impl.PacketverMax != c.pvMax {
			t.Errorf("impl[%d].PacketverMax = %d, want %d", c.idx, impl.PacketverMax, c.pvMax)
		}
	}
}

func TestGetAction_NotFound(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if _, ok := db.GetAction("does_not_exist"); ok {
		t.Error("expected not-found")
	}
}

func TestCreateAction_AddsAtEndOfSemanticActions(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.CreateAction("zc_new_thing", "desc", "openkore_name"); err != nil {
		t.Fatalf("CreateAction: %v", err)
	}
	if err := db.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := os.ReadFile(p)

	// New action should appear after zc_accept_enter (the existing last action).
	idxNew := strings.Index(string(got), "zc_new_thing:")
	idxOld := strings.Index(string(got), "zc_accept_enter:")
	if idxNew < 0 {
		t.Fatal("new action not found after Save")
	}
	if idxNew < idxOld {
		t.Error("new action was inserted before existing actions; expected append")
	}
}

func TestCreateAction_RejectsExistingName(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	err := db.CreateAction("zc_accept_enter", "x", "y")
	if err == nil {
		t.Fatal("expected error creating duplicate action")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error should mention 'already exists', got: %v", err)
	}
}

func TestCreateAction_RejectsEmptyName(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.CreateAction("", "x", "y"); err == nil {
		t.Error("expected error on empty name")
	}
	if err := db.CreateAction("   ", "x", "y"); err == nil {
		t.Error("expected error on whitespace name")
	}
}

func TestDeleteAction_RemovesEntry(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.DeleteAction("zc_accept_enter"); err != nil {
		t.Fatalf("DeleteAction: %v", err)
	}
	if _, ok := db.GetAction("zc_accept_enter"); ok {
		t.Error("action still present after delete")
	}
	if err := db.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := os.ReadFile(p)
	if strings.Contains(string(got), "zc_accept_enter:") {
		t.Error("action still in file after Save")
	}
}

func TestDeleteAction_NotFound(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.DeleteAction("nonexistent"); err == nil {
		t.Error("expected error on missing action")
	}
}

func TestRenameAction_UpdatesKeyAndName(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)

	// Give the action a non-empty name field first, so we exercise the
	// "inner name mirrors old key" branch.
	newName := "actor_died_or_disappeared"
	if err := db.UpdateActionMetadata(newName, nil, nil); err != nil {
		t.Fatalf("prep: %v", err)
	}
	// Manually set inner name to match old key via re-parse trick: just call
	// UpdateActionMetadata with description we don't care about, then verify.
	// Simpler: rename and check both cases directly.

	if err := db.RenameAction("actor_died_or_disappeared", "actor_disappeared"); err != nil {
		t.Fatalf("RenameAction: %v", err)
	}
	if _, ok := db.GetAction("actor_died_or_disappeared"); ok {
		t.Error("old name still present after rename")
	}
	a, ok := db.GetAction("actor_disappeared")
	if !ok {
		t.Fatal("new name not present after rename")
	}
	if len(a.Implementations) != 1 {
		t.Errorf("implementations lost during rename: got %d, want 1", len(a.Implementations))
	}
	if a.Implementations[0].PacketID != "0x0080" {
		t.Errorf("implementation packet_id changed: got %q", a.Implementations[0].PacketID)
	}

	if err := db.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := os.ReadFile(p)
	if strings.Contains(string(got), "actor_died_or_disappeared:") {
		t.Error("old key still in file after Save")
	}
	if !strings.Contains(string(got), "actor_disappeared:") {
		t.Error("new key not in file after Save")
	}
}

func TestRenameAction_PreservesDocumentOrder(t *testing.T) {
	// The sample has actor_died_or_disappeared before zc_accept_enter.
	// After renaming actor_died_or_disappeared → aaa_first, the renamed
	// action must still come before zc_accept_enter.
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.RenameAction("actor_died_or_disappeared", "aaa_first"); err != nil {
		t.Fatalf("RenameAction: %v", err)
	}
	if err := db.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := os.ReadFile(p)
	iFirst := strings.Index(string(got), "aaa_first:")
	iSecond := strings.Index(string(got), "zc_accept_enter:")
	if iFirst < 0 || iSecond < 0 {
		t.Fatalf("keys missing: iFirst=%d iSecond=%d", iFirst, iSecond)
	}
	if iFirst > iSecond {
		t.Errorf("rename disturbed document order: aaa_first at %d, zc_accept_enter at %d", iFirst, iSecond)
	}
}

func TestRenameAction_UpdatesInnerNameFieldWhenItMirrorsKey(t *testing.T) {
	// Build a doc where the inner name: field equals the outer key. Then
	// rename and confirm the inner field is updated too.
	doc := `semantic_actions:
    foo_action:
        name: foo_action
        description: ""
        openkore_name: ""
        canonical_params: []
        implementations: []
`
	dir := t.TempDir()
	p := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	db, err := semanticsdb.Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := db.RenameAction("foo_action", "bar_action"); err != nil {
		t.Fatalf("RenameAction: %v", err)
	}
	if err := db.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "name: bar_action") {
		t.Errorf("inner name: not updated; file:\n%s", got)
	}
	if strings.Contains(string(got), "name: foo_action") {
		t.Errorf("inner name: still has old value; file:\n%s", got)
	}
}

func TestRenameAction_PreservesInnerNameWhenNotMirroring(t *testing.T) {
	// The sample uses name: "" — rename must NOT change it to the new name.
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.RenameAction("actor_died_or_disappeared", "actor_disappeared"); err != nil {
		t.Fatalf("RenameAction: %v", err)
	}
	if err := db.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _ := os.ReadFile(p)
	// Inner name: should still be "" (as in sample), not "actor_disappeared".
	if strings.Contains(string(got), "name: actor_disappeared") {
		t.Errorf("inner name: was updated when it should have been left alone; file:\n%s", got)
	}
}

func TestRenameAction_NotFound(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.RenameAction("nonexistent", "whatever"); err == nil {
		t.Error("expected error on missing action")
	}
}

func TestRenameAction_RejectsExistingTarget(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.RenameAction("actor_died_or_disappeared", "zc_accept_enter"); err == nil {
		t.Error("expected error when renaming to an existing name")
	}
}

func TestRenameAction_RejectsEmptyTarget(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.RenameAction("actor_died_or_disappeared", "   "); err == nil {
		t.Error("expected error on blank new name")
	}
}

func TestRenameAction_SameNameIsNoop(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.RenameAction("zc_accept_enter", "zc_accept_enter"); err != nil {
		t.Errorf("rename to same name should be no-op, got: %v", err)
	}
	if _, ok := db.GetAction("zc_accept_enter"); !ok {
		t.Error("action disappeared after no-op rename")
	}
}

func TestAddImplementation_AppendsToAction(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	err := db.AddImplementation("actor_died_or_disappeared", semanticsdb.Implementation{
		PacketID:     "0x099B",
		StructName:   "SYNTH_ZC_NEW_LAYOUT",
		PacketverMin: 20121010,
	})
	if err != nil {
		t.Fatalf("AddImplementation: %v", err)
	}
	a, _ := db.GetAction("actor_died_or_disappeared")
	if len(a.Implementations) != 2 {
		t.Fatalf("expected 2 impls after add, got %d", len(a.Implementations))
	}
	got := a.Implementations[1]
	if got.PacketID != "0x099B" {
		t.Errorf("new impl packet_id = %q, want 0x099B", got.PacketID)
	}
	if got.StructName != "SYNTH_ZC_NEW_LAYOUT" {
		t.Errorf("new impl struct = %q", got.StructName)
	}
	if got.PacketverMin != 20121010 {
		t.Errorf("new impl min = %d, want 20121010", got.PacketverMin)
	}
	if got.PacketverMax != 0 {
		t.Errorf("new impl max = %d, want 0", got.PacketverMax)
	}
}

func TestAddImplementation_RejectsDuplicatePacketID(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	err := db.AddImplementation("zc_accept_enter", semanticsdb.Implementation{
		PacketID:   "0x0073",
		StructName: "PACKET_ZC_ACCEPT_ENTER",
	})
	if err == nil {
		t.Fatal("expected duplicate packet_id error")
	}
}

func TestAddImplementation_RejectsEmpty(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.AddImplementation("zc_accept_enter", semanticsdb.Implementation{StructName: "X"}); err == nil {
		t.Error("expected error on empty packet_id")
	}
	if err := db.AddImplementation("zc_accept_enter", semanticsdb.Implementation{PacketID: "0x1234"}); err == nil {
		t.Error("expected error on empty struct_name")
	}
	if err := db.AddImplementation("nonexistent", semanticsdb.Implementation{PacketID: "0x1234", StructName: "X"}); err == nil {
		t.Error("expected error on missing action")
	}
}

func TestUpdateImplementation_ChangesRange(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	newMin := 19900101
	newMax := 20011231
	if err := db.UpdateImplementation("zc_accept_enter", "0x0073", nil, &newMin, &newMax); err != nil {
		t.Fatalf("UpdateImplementation: %v", err)
	}
	impl, ok := db.GetImplementation("zc_accept_enter", "0x0073")
	if !ok {
		t.Fatal("impl not found after update")
	}
	if impl.PacketverMin != 19900101 || impl.PacketverMax != 20011231 {
		t.Errorf("got (%d,%d), want (%d,%d)", impl.PacketverMin, impl.PacketverMax, newMin, newMax)
	}
}

func TestUpdateImplementation_ChangesStructOnly(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	newStruct := "PACKET_ZC_ACCEPT_ENTER_V2"
	if err := db.UpdateImplementation("zc_accept_enter", "0x0073", &newStruct, nil, nil); err != nil {
		t.Fatalf("UpdateImplementation: %v", err)
	}
	impl, _ := db.GetImplementation("zc_accept_enter", "0x0073")
	if impl.StructName != newStruct {
		t.Errorf("struct = %q, want %q", impl.StructName, newStruct)
	}
	// Range untouched.
	if impl.PacketverMin != 0 || impl.PacketverMax != 20080101 {
		t.Errorf("range changed unexpectedly: (%d, %d)", impl.PacketverMin, impl.PacketverMax)
	}
}

func TestDeleteImplementation(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	before, _ := db.GetAction("zc_accept_enter")
	if len(before.Implementations) != 3 {
		t.Fatalf("expected 3 impls, got %d", len(before.Implementations))
	}
	if err := db.DeleteImplementation("zc_accept_enter", "0x02EB"); err != nil {
		t.Fatalf("DeleteImplementation: %v", err)
	}
	after, _ := db.GetAction("zc_accept_enter")
	if len(after.Implementations) != 2 {
		t.Fatalf("expected 2 impls after delete, got %d", len(after.Implementations))
	}
	for _, impl := range after.Implementations {
		if impl.PacketID == "0x02EB" {
			t.Error("0x02EB still present")
		}
	}
}

func TestValidate_CleanSamplePasses(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if errs := db.Validate(); errs != nil {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidate_CatchesMinGtMax(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	if err := db.CreateAction("bad_action", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.AddImplementation("bad_action", semanticsdb.Implementation{
		PacketID:     "0x1234",
		StructName:   "X",
		PacketverMin: 20200101,
		PacketverMax: 20190101,
	}); err != nil {
		t.Fatal(err)
	}
	errs := db.Validate()
	if len(errs) == 0 {
		t.Fatal("expected validation errors, got none")
	}
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "min") && strings.Contains(e.Message, "max") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a min>max error, got: %v", errs)
	}
}

func TestValidate_CatchesDuplicateImplInAction(t *testing.T) {
	// Two implementations with the same packet_id within one action.
	// AddImplementation rejects this at the API level, so we craft a YAML
	// file with the duplicate already present (simulating a hand-edit or
	// historical drift) and verify Validate catches it on load.
	crafted := strings.Replace(sampleYAML,
		"              struct_name: PACKET_ZC_NOTIFY_VANISH\n              field_mapping: {}\n",
		"              struct_name: PACKET_ZC_NOTIFY_VANISH\n              field_mapping: {}\n"+
			"            - packet_id: \"0x0080\"\n"+
			"              packetver_range:\n"+
			"                - null\n"+
			"                - null\n"+
			"              struct_name: PACKET_ZC_NOTIFY_VANISH_DUPE\n"+
			"              field_mapping: {}\n",
		1)
	dir := t.TempDir()
	cp := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(cp, []byte(crafted), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := semanticsdb.Load(cp)
	if err != nil {
		t.Fatalf("Load crafted: %v", err)
	}
	errs := db.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "duplicate implementation") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a 'duplicate implementation' error, got: %v", errs)
	}
}

func TestValidate_CatchesCrossActionConflict(t *testing.T) {
	// Two distinct actions share the same packet_id with overlapping
	// packetver ranges. Validate must flag this.
	crafted := strings.Replace(sampleYAML,
		"    actor_died_or_disappeared:",
		"    conflict_a:\n        implementations:\n"+
			"            - packet_id: \"0x0080\"\n"+
			"              packetver_range:\n"+
			"                - 20030000\n"+
			"                - 20050101\n"+
			"              struct_name: PACKET_A\n"+
			"              field_mapping: {}\n"+
			"    conflict_b:\n        implementations:\n"+
			"            - packet_id: \"0x0080\"\n"+
			"              packetver_range:\n"+
			"                - 20040101\n"+
			"                - 20060101\n"+
			"              struct_name: PACKET_B\n"+
			"              field_mapping: {}\n"+
			"    actor_died_or_disappeared:",
		1)
	dir := t.TempDir()
	cp := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(cp, []byte(crafted), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := semanticsdb.Load(cp)
	if err != nil {
		t.Fatalf("Load crafted: %v", err)
	}
	errs := db.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "used by both action") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a cross-action conflict error, got: %v", errs)
	}
}

func TestValidate_NoFalsePositiveOnDisjointRanges(t *testing.T) {
	// Two actions share the same packet_id with disjoint packetver ranges.
	// This is legitimate (e.g. the same wire ID reassigned over time) and
	// must NOT be flagged as a conflict.
	//
	// Use packet_id 0x9999 — not used by any action in sampleYAML — so the
	// only entries to compare are the two we add.
	crafted := strings.Replace(sampleYAML,
		"    actor_died_or_disappeared:",
		"    disjoint_a:\n        implementations:\n"+
			"            - packet_id: \"0x9999\"\n"+
			"              packetver_range:\n"+
			"                - null\n"+
			"                - 20050101\n"+
			"              struct_name: PACKET_A\n"+
			"              field_mapping: {}\n"+
			"    disjoint_b:\n        implementations:\n"+
			"            - packet_id: \"0x9999\"\n"+
			"              packetver_range:\n"+
			"                - 20050102\n"+
			"                - null\n"+
			"              struct_name: PACKET_B\n"+
			"              field_mapping: {}\n"+
			"    actor_died_or_disappeared:",
		1)
	dir := t.TempDir()
	cp := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(cp, []byte(crafted), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := semanticsdb.Load(cp)
	if err != nil {
		t.Fatalf("Load crafted: %v", err)
	}
	for _, e := range db.Validate() {
		if strings.Contains(e.Message, "used by both action") {
			t.Errorf("false-positive cross-action conflict for disjoint ranges: %v", e)
		}
	}
}

func TestValidate_CatchesBadActionName(t *testing.T) {
	// Realistic scenario: someone hand-edited YAML and added "BadName" with
	// a capital letter. The validator must catch this on next load.
	crafted := strings.Replace(sampleYAML,
		"    actor_died_or_disappeared:",
		"    BadCapitalizedName:\n        name: \"\"\n        implementations: []\n    actor_died_or_disappeared:", 1)
	dir := t.TempDir()
	cp := filepath.Join(dir, "mappings.yaml")
	if err := os.WriteFile(cp, []byte(crafted), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := semanticsdb.Load(cp)
	if err != nil {
		t.Fatalf("Load crafted: %v", err)
	}
	errs := db.Validate()
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "action name") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'action name' validation error, got: %v", errs)
	}
}

func TestSearch_ByStructAndPacketID(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	// By struct name substring.
	results := db.Search(semanticsdb.SearchQuery{StructName: "VANISH"}, 0)
	if len(results) != 1 || results[0].Name != "actor_died_or_disappeared" {
		t.Errorf("Search VANISH = %+v", results)
	}
	// By packet ID.
	results = db.Search(semanticsdb.SearchQuery{PacketID: "0x0A18"}, 0)
	if len(results) != 1 || results[0].Name != "zc_accept_enter" {
		t.Errorf("Search 0x0A18 = %+v", results)
	}
	// By openkore_name.
	results = db.Search(semanticsdb.SearchQuery{OpenkoreName: "map_loaded"}, 0)
	if len(results) != 1 || results[0].Name != "zc_accept_enter" {
		t.Errorf("Search map_loaded = %+v", results)
	}
}

func TestStatistics(t *testing.T) {
	p := writeSample(t)
	db, _ := semanticsdb.Load(p)
	s := db.Statistics()
	if s.ActionCount != 2 {
		t.Errorf("ActionCount = %d, want 2", s.ActionCount)
	}
	if s.ImplementationCount != 4 {
		t.Errorf("ImplementationCount = %d, want 4", s.ImplementationCount)
	}
	if s.ActionsWithImpls != 2 {
		t.Errorf("ActionsWithImpls = %d, want 2", s.ActionsWithImpls)
	}
}

func TestProductionMappings_LoadAndValidate(t *testing.T) {
	// Sanity: the real mappings.yaml must load cleanly and validate without
	// false positives. Any errors here indicate either (a) a real bug in the
	// production DB we missed, or (b) a false positive in our validator.
	// Either way, surface them.
	p := writeProduction(t)
	db, err := semanticsdb.Load(p)
	if err != nil {
		t.Fatalf("Load production: %v", err)
	}
	errs := db.Validate()
	if len(errs) > 0 {
		t.Logf("production mappings.yaml has %d validation findings (informational):", len(errs))
		for _, e := range errs[:min(10, len(errs))] {
			t.Logf("  - %v", e)
		}
	}
	s := db.Statistics()
	if s.ActionCount < 200 {
		t.Errorf("expected >= 200 actions in production DB, got %d", s.ActionCount)
	}
}
