# WORKLOG 0061 — Send Encoder Bug Investigation and Fix Plan

**Date**: 2026-03-20
**Status**: COMPLETE (investigation + plan — no code changes yet)
**Reported by**: goKore Epic 40 migration (work logs 0768–0772 in goKore-test)
**Source**: `docs/07_WORK_LOG/0773_2026-03-20_rathena_client_bug_reports.md` in goKore-test

---

## Summary

Four send-direction bugs were reported against v0.4.4. This worklog documents the deep
dive confirming all four are real, expanding the defect count to 13 affected encoder
files (the bug report identified 3; systematic scanning found 10 more with the same root
cause), and laying out a complete fix plan.

No code was changed in this session. This is investigation and planning only.

---

## Bug Report Summary (as filed)

| # | Packet | Function | Root cause claimed |
|---|--------|----------|--------------------|
| 1 | `0x00C5` CZ_ACK_SELECT_DEALTYPE | missing action | No semantic action defined |
| 2 | `0x01D5` CZ_INPUT_EDITDLGSTR | `EncodeNpcTalkText` | Fixed-size `[8]byte`; `copy(p[8:8], ...)` is no-op |
| 4 | `0x00C8`/`0x00C9` CZ_PC_PURCHASE/SELL_ITEMLIST | `EncodeShopBuy`/`EncodeShopSell` | Fixed-size `[4]byte`; `copy(p[4:], ...)` is no-op |
| 5 | `0x017E` CZ_GUILD_CHAT | `ActionGuildChat` send encoder | Encoder exists but not registered |

(Bug #3 was not included in the report — either resolved earlier or not filed.)

---

## Validation: All Four Confirmed Real

### Bug #1 — Missing action for `0x00C5`

rAthena source confirms the struct:

```
// ~/personal/rathena/src/map/packets.hpp:1511–1516
struct PACKET_CZ_ACK_SELECT_DEALTYPE{
    int16 packetType;
    uint32 GID;
    uint8 type;
} __attribute__((packed));
DEFINE_PACKET_HEADER(CZ_ACK_SELECT_DEALTYPE, 0xc5);
```

Wire size: `2 + 4 + 1 = 7 bytes`. Matches `pkg/session/lengths_map.go:103: t[0x00C5] = 7`.

Exhaustive grep across `pkg/` for any mention of `ActionNpcBuySellList`, `NpcBuySell`,
or `0x00C5` (beyond the length table): **zero results**. Action, send struct, encoder,
and registration are all absent.

Correction to bug report naming: rAthena names this `CZ_ACK_SELECT_DEALTYPE`, not
`CZ_NPC_BUY_SELL_ITEMLIST`. The goKore canonical name is `request_buy_sell_list`
(from `goKore/internal/network/packets/ids.go:1627`). The bug report's `ActionNpcBuySellList`
is an invented name; the correct Go constant will be `ActionRequestBuySellList`.

**Verdict: CONFIRMED.**

### Bug #2 — `EncodeNpcTalkText` drops text payload

```
// pkg/encode/npc_talk_text.go
func EncodeNpcTalkText(req send.NpcTalkText, packetver uint32) [8]byte {
    var p [8]byte
    ...
    copy(p[8:8], req.Value)  // BUG: destination is zero-length
    return p
}
```

rAthena struct (`packets.hpp:1836`) has `char value[]` — a C flexible array member.
`TotalSize` for this struct = 8 (fixed header only). Codegen emits `[8]byte`, then
`copy(p[8:8], req.Value)` which has a zero-length destination.

Runtime verified:
```
req := send.NpcTalkText{PacketSize: 14, GID: 12345, Value: "hello"}
result := encode.EncodeNpcTalkText(req, 20180307)
// result is [8]byte; result[8:] == [] (zero bytes)
```

**Verdict: CONFIRMED.**

### Bug #4 — `EncodeShopBuy`/`EncodeShopSell` drop item payload

```
// pkg/encode/shop_buy.go
func EncodeShopBuy(req send.ShopBuy, packetver uint32) [4]byte {
    var p [4]byte
    ...
    copy(p[4:], req.Items)  // BUG: p[4:] is zero-length on a [4]byte
    return p
}
```

rAthena struct (`packets_struct.hpp:3092`):
```c
struct PACKET_CZ_PC_PURCHASE_ITEMLIST {
    int16 packetType;
    int16 packetLength;
    struct PACKET_CZ_PC_PURCHASE_ITEMLIST_sub items[];
} __attribute__((packed));
```

`items[]` is a flex array. `TotalSize` = 4. Codegen emits `[4]byte` with `copy(p[4:], ...)`.

Same pattern in `shop_sell.go`: `PACKET_CZ_PC_SELL_ITEMLIST` (`packets.hpp:298`) has
`PACKET_CZ_PC_SELL_ITEMLIST_sub sellList[]`. `TotalSize` = 4. Same no-op copy.

Runtime verified:
```
req4 := send.ShopBuy{PacketLength: 8, Items: []byte{0x01,0x00,0x03,0x00}}
result := encode.EncodeShopBuy(req4, 20180307)
// result is [4]byte; result[4:] == [] (zero bytes — items dropped)
```

**Verdict: CONFIRMED.**

### Bug #5 — `ActionGuildChat` has no send encoder

```
$ grep "ActionGuildChat" pkg/encode/register.go
# (no output)
```

`pkg/encode/guild_chat.go` exists and correctly implements `EncodeGuildChat`. The action
constant `ActionGuildChat = 149` exists in `pkg/session/actions.go`. The receive entry
`0x017F` is present in `pkg/session/receive_dispatch.go:302`. But `register.go` has
zero `RegisterSendEncoder` entries for `ActionGuildChat` — it is listed in register.go's
own header comment as an action "with no send-direction implementation."

Root cause: `CZ_GUILD_CHAT` (0x017E) has no C struct in rAthena; it is parsed manually
via `clif_process_message` with `whisperFormat=false`. The codegen's `isSendStruct`
filter requires a CZ_ struct name in the DB — since `guild_chat` has only a ZC_ receive
impl (`PACKET_ZC_GUILD_CHAT`), the codegen excludes it from register.go.

**Verdict: CONFIRMED.**

---

## Expanded Defect Count: 13 Files, Not 3

Systematic scan for two bug patterns across all `pkg/encode/*.go` files:

**Pattern 1 — `copy(p[N:N], ...)` (explicit zero-length bounds):**

| File | Pattern | Dropped field |
|------|---------|---------------|
| `npc_talk_text.go` | `copy(p[8:8], req.Value)` | text payload |
| `cz_upload_macro_detector_captcha.go` | `copy(p[8:8], req.ImageData)` | image data |
| `ca_sso_login_req.go` | `copy(p[92:92], req.Token)` | SSO token |

**Pattern 2 — `copy(p[N:], req.X)` where function returns `[N]byte` (implicit zero-length):**

| File | Return type | Pattern | Dropped field |
|------|-------------|---------|---------------|
| `shop_buy.go` | `[4]byte` | `copy(p[4:], req.Items)` | purchase items |
| `shop_sell.go` | `[4]byte` | `copy(p[4:], req.SellList)` | sell list |
| `market_purchase.go` | `[4]byte` | `copy(p[4:], req.List)` | market items |
| `cz_npc_barter_market_purchase.go` | `[4]byte` | `copy(p[4:], req.List)` | barter items |
| `cz_npc_expanded_barter_market_purchase.go` | `[4]byte` | `copy(p[4:], req.List)` | barter items |
| `cz_pc_purchase_itemlist_frommc.go` | `[8]byte` | `copy(p[8:], req.List)` | MC purchase items |
| `cz_pc_purchase_itemlist_frommc2.go` | `[12]byte` | `copy(p[12:], req.List)` | MC purchase items |
| `cz_req_change_memberpos.go` | `[4]byte` | `copy(p[4:], req.List)` | member position list |
| `cz_req_merge_item.go` | `[4]byte` | `copy(p[4:], req.Indices)` | item indices |
| `cz_req_random_combine_item.go` | `[6]byte` | `copy(p[6:], req.Items)` | item list |
| `cz_se_pc_buy_cashitem_list.go` | `[10]byte` | `copy(p[10:], req.Items)` | cash shop items |

All 13 files are generated by the same codegen template (`internal/codegen/gen/encode.go`)
and all have the same root cause.

---

## Root Cause Analysis

### Codegen root cause (bugs #2, #4 and the 10 additional files)

`internal/codegen/gen/encode.go:generateEncodeFunc` (line ~108):

```go
totalSize := layout.TotalSize
returnType := fmt.Sprintf("[%d]byte", totalSize)
if totalSize <= 0 {
    returnType = "[]byte"
}
```

`layout.TotalSize` is the sum of **fixed fields only**. Flex array fields
(`preprocess.Field.IsFlexArray == true`) contribute 0 to `TotalSize` by design
(see `preprocess/parser.go:302: layout.TotalSize = offset` — offset is only
incremented by fixed-size fields; flex array fields have `Size=0`).

Therefore a struct like `PACKET_CZ_PC_PURCHASE_ITEMLIST` (4 fixed bytes + `items[]`)
produces `TotalSize=4`, `returnType="[4]byte"`. The encoder then does:

```go
var p [4]byte
copy(p[4:], req.Items)  // p[4:] is zero-length → no-op
```

The fix: `generateEncodeFunc` must also check whether any layout field has
`IsFlexArray=true`. If so, the packet is variable-length and must return `[]byte`.

**`IsFlexArray` is never checked in `encode.go`** — confirmed by:
```
$ grep -n "IsFlexArray" internal/codegen/gen/encode.go
# (no output)
```

`decode.go` handles `IsFlexArray` correctly (line 340, 387). The same check was
simply never added to the encode path.

The same bug exists in `generateEncodeDispatcher` for the multi-version case
(`commonSize` computation does not account for flex fields).

### Codegen root cause (bug #5 — guild_chat)

`internal/codegen/gen/register.go:generateRegisterFileInner`:

```go
var sendImpls []semantics.Implementation
for _, impl := range action.Implementations {
    if isSendStruct(impl.StructName) {
        sendImpls = append(sendImpls, impl)
    }
}
if len(sendImpls) == 0 {
    skipped = append(skipped, name)
    continue
}
```

`isSendStruct` returns true for struct names beginning with `CZ_`, `CH_`, `CA_`,
`CT_`, `SYNTH_CZ_`, etc. The `guild_chat` action only has `PACKET_ZC_GUILD_CHAT`
(receive direction) in the DB — no CZ_ impl. So `sendImpls` is empty → action
is skipped.

`register.go` scans `pkg/encode/` for existing `Encode*` functions, but the
skip happens *before* that check. `EncodeGuildChat` exists in `pkg/encode/` but
is never reached because the DB filter excludes the action first.

---

## Fix Plan

### Overview

Four fixes, one shared codegen repair:

| Fix | Scope | Trigger |
|-----|-------|---------|
| A | Codegen: `encode.go` — detect flex fields, emit `[]byte` | Fixes all 13 broken encoders |
| B | Codegen: `register.go` — include hand-written encoders without DB send impl | Fixes bug #5 (guild_chat) and similar |
| C | SemanticDB + codegen: add `request_buy_sell_list` action | Fixes bug #1 (0x00C5) |
| Tests | New test files for all affected encoders | Mandatory TDD gate |

Fixes A, B, and C all require a codegen run to take effect on generated files.
Fix A and Fix C can be applied in parallel (different files). Regeneration
happens once after all codegen source changes are made.

---

### Fix A — `internal/codegen/gen/encode.go`: flex-field detection

**Step A1 — Add `hasFlexField` helper** (new unexported function in `encode.go`):

```go
// hasFlexField returns true if any field in the layout is a C flexible array
// member (IsFlexArray=true). Such packets are variable-length and must use
// []byte return, not [N]byte.
func hasFlexField(layout *preprocess.StructLayout) bool {
    for i := range layout.Fields {
        if layout.Fields[i].IsFlexArray {
            return true
        }
    }
    return false
}
```

**Step A2 — `generateEncodeFunc`: change return-type decision**

Before (line ~108):
```go
totalSize := layout.TotalSize
returnType := fmt.Sprintf("[%d]byte", totalSize)
if totalSize <= 0 {
    returnType = "[]byte"
}
```

After:
```go
totalSize := layout.TotalSize
isVariable := totalSize <= 0 || hasFlexField(layout)
returnType := "[]byte"
if !isVariable {
    returnType = fmt.Sprintf("[%d]byte", totalSize)
}
```

**Step A3 — `generateEncodeFunc`: change allocation for variable packets with flex fields**

The existing `totalSize <= 0` path emits:
```go
p := make([]byte, 4)
```

This is wrong for flex-array packets (where `totalSize > 0`, e.g. 4 for shop_buy).
The allocation must be:
```go
p := make([]byte, totalSize+len(req.<FlexFieldName>))
```

Find the flex field name at generation time:
```go
// flexFieldGoName returns the Go identifier of the first IsFlexArray field,
// or "" if none.
func flexFieldGoName(layout *preprocess.StructLayout) string {
    for i := range layout.Fields {
        if layout.Fields[i].IsFlexArray {
            return cFieldToGoIdent(layout.Fields[i].Name)
        }
    }
    return ""
}
```

Then in the allocation block:
```go
if !isVariable {
    sb.WriteString(fmt.Sprintf("\tvar p %s\n", returnType))
} else if flexName := flexFieldGoName(layout); flexName != "" {
    sb.WriteString(fmt.Sprintf("\tp := make([]byte, %d+len(req.%s))\n", totalSize, flexName))
} else {
    sb.WriteString("\tp := make([]byte, 4)\n")  // legacy path for TotalSize <= 0
}
```

**Step A4 — `generateEncodeDispatcher`: apply same flex-field logic**

The `commonSize` computation (line ~181):
```go
commonSize := -1
for i := range impls {
    layout := resolveLayout(...)
    if layout == nil { continue }
    if layout.TotalSize <= 0 { commonSize = 0; break }
    ...
}
```

Add after `layout.TotalSize <= 0` check:
```go
if hasFlexField(layout) { commonSize = 0; break }
```

Inside the per-impl case block, replace `make([]byte, 4)` with the flex-aware allocation:
```go
if totalSize > 0 && !hasFlexField(layout) {
    sb.WriteString(fmt.Sprintf("\t\tp := make([]byte, %d)\n", totalSize))
} else if flexName := flexFieldGoName(layout); flexName != "" {
    sb.WriteString(fmt.Sprintf("\t\tp := make([]byte, %d+len(req.%s))\n", totalSize, flexName))
} else {
    sb.WriteString("\t\tp := make([]byte, 4)\n")
}
```

**Step A5 — `internal/codegen/gen/send.go`: remove `packetLength`/`packetSize` from flex-array send structs**

Currently `sendFields` includes all fields from the struct, including `packetLength`
and `packetSize`. For variable-payload packets these should be computed by the encoder,
not passed by callers. Update `sendFields` to skip the length/size field when the struct
has a flex array:

```go
func sendFields(a *semantics.Action, vt preprocess.VersionTable) []eventField {
    ...
    for i := range layout.Fields {
        f := &layout.Fields[i]
        if f.Name == "PacketType" || f.Name == "packetType" || f.Name == "packet_type" {
            continue
        }
        // For structs with flex array payloads, the encoder computes packetLength/
        // packetSize internally — do not expose it as a caller-visible field.
        if hasFlexField(layout) && isLengthField(f.Name) {
            continue
        }
        ...
    }
}

// isLengthField returns true for C field names that represent the packet's own
// wire length (computed by the encoder, not set by the caller).
func isLengthField(name string) bool {
    switch strings.ToLower(name) {
    case "packetlength", "packetlen", "packetsize", "length", "len":
        return true
    }
    return false
}
```

The encoder must then compute and write the length field itself using `uint16(totalSize + len(req.FlexField))`.

**Breaking change note:** Removing `PacketLength`/`PacketSize` from send structs is a
breaking API change. The goKore consumer currently bypasses all 13 affected encoders with
raw-bytes workarounds, so the break is expected and acceptable. Document in CHANGELOG.

---

### Fix B — `internal/codegen/gen/register.go`: include hand-written encoders

The register generator currently skips actions with no DB send-direction impl
(`len(sendImpls) == 0`). This causes `guild_chat` and any other action with a
hand-written encoder but no DB CZ_ impl to be silently excluded.

**Change `generateRegisterFileInner`** to check the encode dir before deciding to skip:

```go
// Current (skips before checking encode dir):
if len(sendImpls) == 0 {
    skipped = append(skipped, name)
    continue
}

// Fixed:
encodeFuncName := fmt.Sprintf("Encode%s", goIdent)
hasHandWrittenEncoder := encodeDir != "" && existingEncoders[encodeFuncName]
if len(sendImpls) == 0 && !hasHandWrittenEncoder {
    skipped = append(skipped, name)
    continue
}
```

When `hasHandWrittenEncoder` is true and `sendImpls` is empty, the function is
included in register.go as a variable-length entry (`isFixed = false`) because
`commonSize` will be -1 (no DB layouts) and `fixedReturnEncoders[encodeFuncName]`
will be false (hand-written function returns `[]byte`).

**Side effect check:** This would also pick up `EncodeBattleChat`, `EncodePartyChat`,
`EncodeWhisper` which are also hand-written `[]byte` encoders not currently registered.
Verify each has a corresponding `ActionX` constant before applying — if not, gate the
inclusion behind an explicit allowlist in the codegen (a `handWrittenSendActions` map
analogous to `fsmOwnedActions`).

**Immediate safe option:** Add `guild_chat` to an explicit `handWrittenSendActions`
whitelist in `register.go` codegen, and expand the list as other hand-written encoders
are verified. This is more conservative than the general fix above.

---

### Fix C — SemanticDB: add `request_buy_sell_list` for `0x00C5`

**GCC verification:**
```bash
g++ -E -P -DPACKETVER=20180307 -DPACKETVER_MAIN_NUM=20180307 \
    -I ~/personal/rathena/src -I ~/personal/rathena/src/map -I ~/personal/rathena/src/common \
    -include internal/codegen/stubs/packets_hpp_stub.h \
    ~/personal/rathena/src/map/packets.hpp 2>/dev/null \
    | grep -A 6 "struct PACKET_CZ_ACK_SELECT_DEALTYPE"
```

Expected output (confirmed from rAthena source read):
```c
struct PACKET_CZ_ACK_SELECT_DEALTYPE{
    int16 packetType;
    uint32 GID;
    uint8 type;
} __attribute__((packed));
```

Wire size: `2 + 4 + 1 = 7 bytes`. No flex array. Fixed return `[7]byte` is correct.

**SemanticDB action to add via MCP** (`gokore-semantics`):
- Action: `request_buy_sell_list`
- Description: `"Open NPC buy or sell dialog — select buy (type=1) or sell (type=0)"`
- OpenKore name: `request_buy_sell_list`
- Implementation: `packet_id=0x00C5`, `struct_name=PACKET_CZ_ACK_SELECT_DEALTYPE`,
  `packetver_range=[null, null]`

**Generated output after codegen run:**
- `pkg/send/request_buy_sell_list.go`:
  ```go
  type RequestBuySellList struct {
      GID  uint32
      Type uint8
  }
  ```
  (Note: `PacketType` is skipped by codegen; `type` → `Type` by `cFieldToGoIdent`.
  Bug report suggested `GID uint32` — rAthena uses `uint32 GID` so this is correct.
  Bug report suggested `BuyOrSell uint8` — codegen will use the rAthena field name
  `Type` not `BuyOrSell`.)
- `pkg/encode/request_buy_sell_list.go`:
  ```go
  func EncodeRequestBuySellList(req send.RequestBuySellList, packetver uint32) [7]byte {
      var p [7]byte
      p[0] = 0xc5; p[1] = 0x00
      leU32Put(p[2:], req.GID)   // rAthena: GID
      p[6] = req.Type            // rAthena: type
      _ = packetver
      return p
  }
  ```
- `ActionRequestBuySellList SemanticAction = 449` in `pkg/session/actions.go`
- One `RegisterSendEncoder` entry in `pkg/encode/register.go`

**Placement in actions.go:** New constant appended at 449 (after current max 448).
Constants are explicitly numbered (not iota), so appending is safe — no existing
constant values change.

---

### Implementation Sequence

```
Step 1  Write failing tests (TDD gate — tests must fail before code changes)
Step 2  Fix A: patch internal/codegen/gen/encode.go (hasFlexField + allocation + send)
Step 3  Fix B: patch internal/codegen/gen/register.go (hand-written encoder inclusion)
Step 4  Fix C: add request_buy_sell_list to semanticDB via MCP
Step 5  Run codegen (single pass — regenerates all affected files)
Step 6  Verify: go build ./... && go test ./...
Step 7  Verify: benchmarks on affected encoders (expect 1 alloc/op for make())
Step 8  Work log
```

Fixes A, B, C all feed into the same codegen run. No fix requires more than one
regeneration pass.

---

### Tests Required (TDD — write before any code)

All test files in `pkg/encode/`, matching the existing pattern in `move_to_test.go`
and `actor_action_test.go`.

**New test files:**

| File | Key tests |
|------|-----------|
| `pkg/encode/shop_buy_test.go` | `TestEncodeShopBuy_ItemsWritten` — items bytes present at offset 4; `TestEncodeShopBuy_TotalLength` — len == 4+len(items); `TestEncodeShopBuy_LengthFieldComputed` — bytes 2-3 == total len; `TestEncodeShopBuy_Empty` — 4 bytes when items nil; `BenchmarkEncodeShopBuy` |
| `pkg/encode/shop_sell_test.go` | Same pattern for sell list |
| `pkg/encode/npc_talk_text_test.go` | `TestEncodeNpcTalkText_TextWritten` — "hello\x00" at offset 8; `TestEncodeNpcTalkText_LengthField` — bytes 2-3 == 8+len(text)+1; `TestEncodeNpcTalkText_NulTerminator`; `TestEncodeNpcTalkText_EmptyString` — 9 bytes (header + NUL only); `BenchmarkEncodeNpcTalkText` |
| `pkg/encode/guild_chat_test.go` | `TestEncodeGuildChat_SendRegistered` — `session.Send(ms, conn, ActionGuildChat, ...)` returns no error; `TestEncodeGuildChat_WireFormat` — wire bytes match expected format |
| `pkg/encode/request_buy_sell_list_test.go` | `TestEncodeRequestBuySellList_GoldenBytes` — 7-byte output, ID=0xC500, GID at bytes 2-5, Type at byte 6; `TestEncodeRequestBuySellList_BuyFlag`; `TestEncodeRequestBuySellList_SellFlag`; `BenchmarkEncodeRequestBuySellList` (0 allocs/op — fixed-size) |

**Tests for the 10 additional affected encoders:**
Each of the files in the expanded defect list needs at minimum:
- `TestEncodeX_PayloadWritten` — payload bytes are non-zero in the output
- `TestEncodeX_TotalLength` — output length equals header + payload length

**Benchmark targets:**
- Fixed-size encoders (`EncodeRequestBuySellList`, etc.): `0 allocs/op` (unchanged)
- Variable-length encoders (`EncodeShopBuy`, `EncodeNpcTalkText`, etc.): `1 allocs/op`
  from the `make([]byte, ...)` call. This is unavoidable and expected for variable-length
  packets. Document in benchmark output and README-LLM.md performance contract table.

---

## Files Affected by Fix Plan

### Codegen source (patched manually):
- `internal/codegen/gen/encode.go` — Fix A
- `internal/codegen/gen/send.go` — Fix A (remove length field from flex-array structs)
- `internal/codegen/gen/register.go` — Fix B

### Generated files (regenerated after codegen fix):
- `pkg/encode/shop_buy.go`
- `pkg/encode/shop_sell.go`
- `pkg/encode/npc_talk_text.go`
- `pkg/encode/market_purchase.go`
- `pkg/encode/cz_npc_barter_market_purchase.go`
- `pkg/encode/cz_npc_expanded_barter_market_purchase.go`
- `pkg/encode/cz_pc_purchase_itemlist_frommc.go`
- `pkg/encode/cz_pc_purchase_itemlist_frommc2.go`
- `pkg/encode/cz_req_change_memberpos.go`
- `pkg/encode/cz_req_merge_item.go`
- `pkg/encode/cz_req_random_combine_item.go`
- `pkg/encode/cz_se_pc_buy_cashitem_list.go`
- `pkg/encode/cz_upload_macro_detector_captcha.go`
- `pkg/encode/ca_sso_login_req.go`
- `pkg/encode/register.go` — gains guild_chat + request_buy_sell_list entries
- `pkg/session/actions.go` — gains ActionRequestBuySellList = 449
- `pkg/send/shop_buy.go` — PacketLength field removed
- `pkg/send/shop_sell.go` — PacketLength field removed
- `pkg/send/npc_talk_text.go` — PacketSize field removed
- `pkg/send/request_buy_sell_list.go` — new file
- `pkg/encode/request_buy_sell_list.go` — new file

### New send struct (generated by Fix C + codegen):
- `pkg/send/request_buy_sell_list.go`

### New encoder (generated by Fix C + codegen):
- `pkg/encode/request_buy_sell_list.go`

### New test files (hand-written, before codegen run):
- `pkg/encode/shop_buy_test.go`
- `pkg/encode/shop_sell_test.go`
- `pkg/encode/npc_talk_text_test.go`
- `pkg/encode/guild_chat_test.go`
- `pkg/encode/request_buy_sell_list_test.go`
- (plus tests for the 10 additional affected encoders)

---

## Open Questions for Implementation

1. **Fix B scope:** Should the register codegen include *all* hand-written encoders
   found in `pkg/encode/` with no DB send impl (general fix), or only an explicit
   whitelist? The general fix would also register `EncodeBattleChat`, `EncodePartyChat`,
   `EncodeWhisper` — verify these are intentionally send-capable before expanding.

2. **`PacketSize` in `NpcTalkText`:** The bug report proposes removing `PacketSize` from
   `send.NpcTalkText`. Fix A's `isLengthField` approach would handle this automatically.
   Confirm the rAthena `packetSize` field in `PACKET_CZ_INPUT_EDITDLGSTR` should be
   computed by the encoder (yes — it is the total wire length, not semantic data).

3. **`ca_sso_login_req` Token field:** The SSO token (`copy(p[92:92], req.Token)`) may
   have a fixed max length (checked via rAthena struct). If `char token[]` is truly a
   flex array, Fix A applies. If it has a bounded length defined by a constant, the
   field would be `char token[N]` (fixed array, not flex array) and Fix A would not
   apply — the bug would be in the field size or offset calculation instead. Verify
   the preprocessed struct before implementing.

---

## Cross-Reference

- Bug reports source: `~/personal/goKore-test/docs/07_WORK_LOG/0773_2026-03-20_rathena_client_bug_reports.md`
- Related EPIC (does NOT cover these bugs): `docs/BACKLOG/EPIC-08_missing_packet_coverage.md`
- Implementation will be tracked in a separate worklog (0062+)
