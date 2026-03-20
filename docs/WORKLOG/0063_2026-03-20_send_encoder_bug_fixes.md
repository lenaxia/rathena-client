# WORKLOG 0063 — Send Encoder Bug Fixes (Bugs #1, #2, #4, #5)

**Date**: 2026-03-20
**Status**: COMPLETE
**Follows**: WORKLOG 0061 (investigation and plan)
**Bug source**: `~/personal/goKore-test/docs/07_WORK_LOG/0773_2026-03-20_rathena_client_bug_reports.md`

---

## Summary

Fixed all 4 confirmed send-direction bugs from the goKore Epic 40 migration report,
plus 10 additional encoders with the same root-cause defect discovered during the
deep dive. Total affected files fixed: 13 generated encoders + 13 send structs.

The fix is implemented as three codegen patches (no manual file edits on generated
code), a semantic DB addition, and a one-time codegen regeneration pass.

---

## What Was Fixed

### Bugs from the report

| Bug | Root cause | Fix |
|-----|-----------|-----|
| #1 — Missing `ActionRequestBuySellList` (0x00C5) | Action not in semanticDB | Added `request_buy_sell_list` action via MCP; codegen regenerated |
| #2 — `EncodeNpcTalkText` drops text payload | `[8]byte` return; `copy(p[8:8],...)` no-op | Fix A: codegen now detects flex fields → `[]byte` return |
| #4 — `EncodeShopBuy`/`EncodeShopSell` drop items | `[4]byte` return; `copy(p[4:],...)` no-op | Fix A: same |
| #5 — `ActionGuildChat` not registered as send encoder | `register.go` codegen skipped actions with no DB send impl | Fix B: codegen now includes hand-written encoders |

### Additional encoders with same defect (found during investigation)

All fixed by Fix A in the same codegen run:

| File | Was | Now |
|------|-----|-----|
| `pkg/encode/market_purchase.go` | `[4]byte`, `copy(p[4:],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/cz_npc_barter_market_purchase.go` | `[4]byte`, `copy(p[4:],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/cz_npc_expanded_barter_market_purchase.go` | `[4]byte`, `copy(p[4:],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/cz_pc_purchase_itemlist_frommc.go` | `[8]byte`, `copy(p[8:],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/cz_pc_purchase_itemlist_frommc2.go` | `[12]byte`, `copy(p[12:],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/cz_req_change_memberpos.go` | `[4]byte`, `copy(p[4:],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/cz_req_merge_item.go` | `[4]byte`, `copy(p[4:],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/cz_req_random_combine_item.go` | `[6]byte`, `copy(p[6:],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/cz_se_pc_buy_cashitem_list.go` | `[10]byte`, `copy(p[10:],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/cz_upload_macro_detector_captcha.go` | `[8]byte`, `copy(p[8:8],...)` no-op | `[]byte`, dynamic alloc |
| `pkg/encode/ca_sso_login_req.go` | `[92]byte`, `copy(p[92:92],...)` no-op | `[]byte`, dynamic alloc |

---

## GCC Verification

### `PACKET_CZ_ACK_SELECT_DEALTYPE` (Bug #1)

```
Command: g++ -E -P -DPACKETVER=20180307 ... packets.hpp | grep -A 6 "PACKET_CZ_ACK_SELECT_DEALTYPE"
Output:
  struct PACKET_CZ_ACK_SELECT_DEALTYPE{
      int16 packetType;
      uint32 GID;
      uint8 type;
  } __attribute__((packed));
  DEFINE_PACKET_HEADER(CZ_ACK_SELECT_DEALTYPE, 0xc5);
Wire size: 2 + 4 + 1 = 7 bytes. No flex array. Fixed [7]byte return is correct. ✓
```

### `PACKET_CZ_INPUT_EDITDLGSTR` (Bug #2)

```
Command: g++ -E -P -DPACKETVER=20180307 ... packets.hpp | grep -A 8 "PACKET_CZ_INPUT_EDITDLGSTR"
Output:
  struct PACKET_CZ_INPUT_EDITDLGSTR{
      int16 packetType;
      uint16 packetSize;
      int32 GID;
      char value[];
  } __attribute__((packed));
Flex field: char value[] (IsFlexArray=true). TotalSize = 8. Fix: []byte return,
make([]byte, 8+len(req.Value)), leU16Put(p[2:], uint16(len(p))) computed internally. ✓
```

### `PACKET_CZ_PC_PURCHASE_ITEMLIST` and `PACKET_CZ_PC_SELL_ITEMLIST` (Bug #4)

Both verified with GCC at PACKETVER=20180307 — both have flex array struct members
(`items[]` and `sellList[]` respectively) with TotalSize=4. Fixed to `[]byte` return
with dynamic allocation. ✓

---

## Codegen Changes

### `internal/codegen/gen/encode.go`

**New helpers (added before `generateEncodeFunc`):**

1. `hasFlexField(layout) bool` — returns true if any field has `IsFlexArray=true`
2. `flexFieldGoName(layout) string` — returns the Go identifier of the first flex field
3. `isLengthField(name) bool` — returns true for `packetlength`, `packetlen`, `packetsize`

**`generateEncodeFunc` changes:**

- Return type: `isVariable := totalSize <= 0 || hasFlexField(layout)` — if variable, use `[]byte`
- Allocation: `make([]byte, totalSize+len(req.<FlexFieldName>))` for flex-field packets
- Length field: when `isVariable && isLengthField(f.Name)`, emit `leU16Put(p[off:], uint16(len(p)))` instead of reading from `req.PacketLength`

**`generateEncodeDispatcher` changes:**

- `commonSize` loop: `hasFlexField(layout)` forces `commonSize = 0` (breaks to `[]byte`)
- Per-impl allocation: same flex-aware allocation as `generateEncodeFunc`
- Length field: same inline computation

**`fieldWriteStmt` changes:**

- `string` case: added `f.IsFlexArray` check first → `copy(p[off:], req.X)` with open
  end, preventing the `copy(p[off:off+0], ...)` zero-length pattern for `char[]` flex fields

### `internal/codegen/gen/send.go`

**`sendFields` changes:**

- Added `flex := hasFlexField(layout)`
- When `flex && isLengthField(f.Name)`: skip the field entirely. The `packetLength`/
  `packetSize` field is removed from the send struct — the encoder computes it internally.

### `internal/codegen/gen/register.go`

**`generateRegisterFileInner` changes:**

- After collecting `sendImpls`, compute `encodeFuncName` and check `existingEncoders`
- `hasHandWrittenEncoder := encodeDir != "" && existingEncoders[encodeFuncName]`
- Skip only when `len(sendImpls) == 0 && !hasHandWrittenEncoder`
- This allows `guild_chat`/`EncodeGuildChat` (hand-written, no DB send impl) to be
  included in register.go

---

## SemanticDB Changes (via MCP)

Added new action `request_buy_sell_list`:
- Description: `"Open NPC buy or sell dialog — client selects buy (Type=1) or sell (Type=0)"`
- OpenKore name: `request_buy_sell_list`
- Implementation: `packet_id=0x00C5`, `struct_name=PACKET_CZ_ACK_SELECT_DEALTYPE`,
  `packetver_range=[null, null]`

---

## Generated Files Changed

### New files

- `pkg/send/request_buy_sell_list.go` — `type RequestBuySellList struct { GID uint32; Type uint8 }`
- `pkg/encode/request_buy_sell_list.go` — `func EncodeRequestBuySellList(...) [7]byte`

### Modified files (regenerated)

**send structs** — `PacketLength`/`PacketSize` field removed:
- `pkg/send/shop_buy.go`
- `pkg/send/shop_sell.go`
- `pkg/send/npc_talk_text.go`
- `pkg/send/market_purchase.go`
- `pkg/send/cz_npc_barter_market_purchase.go`
- `pkg/send/cz_npc_expanded_barter_market_purchase.go`
- `pkg/send/cz_pc_purchase_itemlist_frommc.go`
- `pkg/send/cz_pc_purchase_itemlist_frommc2.go`
- `pkg/send/cz_req_change_memberpos.go`
- `pkg/send/cz_req_merge_item.go`
- `pkg/send/cz_req_random_combine_item.go`
- `pkg/send/cz_se_pc_buy_cashitem_list.go`
- `pkg/send/cz_upload_macro_detector_captcha.go`
- `pkg/send/ca_sso_login_req.go`

**encode functions** — return type changed to `[]byte`, payload now written correctly:
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

**session/actions.go** — `ActionRequestBuySellList = 207` added (alphabetical insertion;
all subsequent constants renumbered accordingly; no semantic change since all values are
explicit and all code uses constant names, not integer literals)

**encode/register.go** — two new `RegisterSendEncoder` entries:
- `ActionGuildChat` → `EncodeGuildChat` (hand-written, `[]byte` return)
- `ActionRequestBuySellList` → `EncodeRequestBuySellList` (`[7]byte` return)

---

## New Tests

**`pkg/encode/shop_buy_test.go`**
- `TestEncodeShopBuy_ItemsWritten` ← was failing, now passes
- `TestEncodeShopBuy_PacketID`, `_TotalLength`, `_LengthFieldComputed`, `_EmptyItems`, `_MultipleItems`
- `BenchmarkEncodeShopBuy`

**`pkg/encode/shop_sell_test.go`**
- `TestEncodeShopSell_SellListWritten` ← was failing, now passes
- `TestEncodeShopSell_PacketID`, `_TotalLength`, `_LengthFieldComputed`, `_EmptySellList`, `_MultipleEntries`
- `BenchmarkEncodeShopSell`

**`pkg/encode/npc_talk_text_test.go`**
- `TestEncodeNpcTalkText_TextWritten` ← was failing, now passes
- `TestEncodeNpcTalkText_PacketID`, `_GIDField`, `_LengthField`, `_TotalLength`, `_EmptyString`, `_Unicode`
- `BenchmarkEncodeNpcTalkText`

**`pkg/encode/guild_chat_test.go`**
- `TestEncodeGuildChat_WireFormat`
- `TestEncodeGuildChat_SendRegistered` ← was failing, now passes
- `BenchmarkEncodeGuildChat`

**`pkg/encode/request_buy_sell_list_test.go`**
- `TestEncodeRequestBuySellList_GoldenBytes`, `_BuyFlag`, `_SellFlag`, `_GIDZero`, `_GIDMax`, `_PacketverIndependent`
- `BenchmarkEncodeRequestBuySellList`

---

## Test Results

```
$ go test ./...
ok  github.com/lenaxia/rathena-client/internal/codegen/gen    0.009s
ok  github.com/lenaxia/rathena-client/internal/codegen/preprocess
ok  github.com/lenaxia/rathena-client/internal/codegen/semantics
ok  github.com/lenaxia/rathena-client/pkg/decode
ok  github.com/lenaxia/rathena-client/pkg/encode              0.009s
ok  github.com/lenaxia/rathena-client/pkg/packing
ok  github.com/lenaxia/rathena-client/pkg/session             0.161s

$ go test -race ./...
# All PASS

$ grep -r "^\s*go " pkg/
# (empty — zero goroutines in pkg/)
```

---

## Benchmark Results

```
BenchmarkEncodeRequestBuySellList    1000000000   0.19 ns/op   0 B/op   0 allocs/op
BenchmarkEncodeShopBuy               1000000000   0.17 ns/op   0 B/op   0 allocs/op  *
BenchmarkEncodeShopSell              1000000000   0.18 ns/op   0 B/op   0 allocs/op  *
BenchmarkEncodeNpcTalkText           1000000000   0.24 ns/op   0 B/op   0 allocs/op  *
BenchmarkEncodeGuildChat              34644750   29.51 ns/op   0 B/op   0 allocs/op  *

* When result is discarded (_ = result), compiler eliminates the make() allocation.
  With a live sink: 1 allocs/op @ ~23 ns/op (e.g. BenchmarkEncodeShopBuy_Payload).
  This is the expected and correct result for variable-length packets.

BenchmarkFeed_SmallFixedPacket       37386050   26.83 ns/op   0 B/op   0 allocs/op
BenchmarkFeed_ActorExists_0x09FF     52465215   21.87 ns/op   0 B/op   0 allocs/op
BenchmarkEncode_RequestMove         1000000000    0.53 ns/op   0 B/op   0 allocs/op
```

All performance contract targets met. `EncodeRequestBuySellList` is `[7]byte` fixed return
and achieves 0 allocs/op as expected.

---

## Breaking Change Note

Removing `PacketLength`/`PacketSize` from send structs is a breaking API change. The
goKore consumer is not affected in practice because:
1. It was bypassing all affected encoders with raw-bytes workarounds (the bugs made
   the encoded output unusable)
2. When it migrates to use `session.Send()`, it will construct the send structs
   without the removed fields (encoders compute the length internally)

The workaround builders in `goKore/internal/network/send/builders/` can now be removed
and replaced with `session.Send()` calls.

---

## Validation Scripts

```
$ ./validation/preprocess_check.sh 20180307
packets_struct.hpp ... OK (770 structs)
packets.hpp ... OK (641 structs)
common/packets.hpp ... OK (131 structs)
clif_obfuscation.hpp ... OK (1 key definitions)
All headers preprocessed successfully.
```
