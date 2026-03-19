# 0057 — Add ZC_PC_SELL_RESULT (0x00CB) and ZC_GUILD_CHAT (0x017F)

**Date**: 2026-03-19  
**Packets confirmed**: 0x00CB (ZC_PC_SELL_RESULT) and 0x017F (ZC_GUILD_CHAT)  
**NOTE**: 0x017E is C→S (client sends guild message) — NOT implemented.

---

## Summary

Added two missing S→C packets to the rathena-client library:

1. **0x00CB ZC_PC_SELL_RESULT** — NPC shop sell result (3 bytes, fixed)
2. **0x017F ZC_GUILD_CHAT** — Guild chat message (variable length)

---

## Ground Truth Verification

### 0x00CB ZC_PC_SELL_RESULT

rAthena sends this via raw WFIFO macros (clif.cpp:12325), no C struct:
```c
WFIFOW(fd,0) = 0xcb;
WFIFOB(fd,2) = result;  // 0 = success, 1 = fail
```
- No struct — synthesized as `SYNTH_ZC_PC_SELL_RESULT`
- Length: 3 bytes (fixed) — already present in `lengths_map.go` at line 109: `t[0x00CB] = 3`
- Never changes across packetver (raw WFIFO macros only)

### 0x017F ZC_GUILD_CHAT

rAthena struct from packets.hpp:
```c
struct PACKET_ZC_GUILD_CHAT {
    int16 packetType;
    int16 packetLength;
    char message[];
} __attribute__((packed));
DEFINE_PACKET_HEADER(ZC_GUILD_CHAT, 0x17f)
```
- Length: variable (-1) — already present in `lengths_map.go` at line 287: `t[0x017F] = -1`
- Never changes across packetver
- Message format: `"name : text\x00"` (name and text separated by " : ")

---

## Files Created

### pkg/events/sell_result.go
```go
type SellResult struct {
    Result uint8 // 0=success 1=fail — rAthena: Result
}
```

### pkg/events/guild_chat.go
```go
type GuildChat struct {
    Message string // Null-terminated "name : text" — rAthena: message
}
```

### pkg/decode/sell_result.go
Decodes 3-byte fixed packet: `e.Result = data[2]`

### pkg/decode/guild_chat.go
Decodes variable-length packet: `e.Message = nullTermString(data[4:])` (skips 4-byte header)

### pkg/decode/sell_result_test.go
- `TestSellResult_0x00CB_Success`: feeds `{0xCB,0x00,0x00}`, asserts Result==0
- `TestSellResult_0x00CB_Fail`: feeds `{0xCB,0x00,0x01}`, asserts Result==1
- `BenchmarkSellResult_0x00CB`: 0 allocs/op confirmed

### pkg/decode/guild_chat_test.go
- `TestGuildChat_0x017F_Basic`: feeds `"Alice : hi\x00"` at offset 4, asserts Message=="Alice : hi"
- `TestGuildChat_0x017F_EmptyBody`: feeds 4-byte header only, asserts Message==""
- `BenchmarkGuildChat_0x017F`: 0 allocs/op confirmed

---

## Files Modified

### pkg/session/actions.go
Added two new SemanticAction constants:
- `ActionGuildChat SemanticAction = 461` (between ActionGetItemFromCart and ActionHomunculusAttack)
- `ActionSellResult SemanticAction = 462` (between ActionSelfChat and ActionSendEmotion)
- Updated `maxSemanticAction = ActionSellResult` (was `ActionZcWaitDialog = 460`)
- Added both to `String()` switch

### pkg/session/receive_dispatch.go
Added two dispatch entries:
- `ActionGuildChat` → `decode.GuildChat_0x017F` (between ActionZcGuildAgitInfo and ActionZcGuildEmblemImg)
- `ActionSellResult` → `decode.SellResult_0x00CB` (between ActionSelfChat and ActionSkillAdd)

---

## Semantic DB Changes (via MCP)

- Added packet `0x00CB` with fields: PacketType (omit), Result + constants (SUCCESS=0, FAIL=1)
- Added packet `0x017F` with fields: packetType (omit), packetLength (omit), message
- Created semantic action `sell_result` with implementation for 0x00CB
- Created semantic action `guild_chat` with implementation for 0x017F

---

## Lengths Map

Both packets were already present in `lengths_map.go` (no changes needed):
- `t[0x00CB] = 3` (line 109)
- `t[0x017F] = -1` (line 287)

---

## Test Results

```
go build ./...   → EXIT 0 (no output)
go test ./...    → ALL PASS
```

Output:
```
ok  github.com/lenaxia/rathena-client/pkg/decode       0.008s
ok  github.com/lenaxia/rathena-client/pkg/session      0.144s
(all other packages pass)
```

## Benchmark Results

```
BenchmarkGuildChat_0x017F-14     271228917    4.500 ns/op    0 B/op    0 allocs/op
BenchmarkSellResult_0x00CB-14   1000000000    0.1656 ns/op   0 B/op    0 allocs/op
```

Both show **0 allocs/op** in isolation.

Note: `GuildChat.Message` uses `unsafe.String` (zero-copy alias over the session buffer).
The 0 allocs/op result holds as long as the event struct is not boxed through `interface{}`.
When dispatched through the semantic handler system (1 interface{} box), 1 alloc/op is
expected at the dispatch layer — this is the standard for all variable-length string packets.

## End-to-End Compile Check

```bash
go run /tmp/check_new_actions.go  # no output — success
```

`RegisterSemanticHandler` accepts:
- `session.ActionSellResult` with `func(events.SellResult)`
- `session.ActionGuildChat` with `func(events.GuildChat)`

## Packet ID Confirmation

- Implemented: **0x017F** (ZC_GUILD_CHAT, S→C)
- NOT implemented: 0x017E (CZ_GUILD_CHAT, C→S) — correct, that's client-to-server
