#!/usr/bin/env bash
set -euo pipefail

RATHENA="${RATHENA_ROOT:-$HOME/personal/rathena}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
STUBS="$REPO/validation/stubs"
OUT="$REPO/validation/output/gate"
REPORT="$REPO/validation/phase1_gate_report.md"

PACKETVER_MODERN="20180307"
PACKETVER_OLD="20160101"

mkdir -p "$OUT"

echo "# Phase 1 Gate Verification Report" > "$REPORT"
echo "" >> "$REPORT"
echo "**Generated**: $(date -Iseconds)" >> "$REPORT"
echo "**rAthena path**: $RATHENA" >> "$REPORT"
echo "**PACKETVER**: $PACKETVER_MODERN (modern), $PACKETVER_OLD (old)" >> "$REPORT"
echo "" >> "$REPORT"

PASS=0
FAIL=0

record() {
    local status="$1"
    local section="$2"
    local msg="$3"
    if [ "$status" == "PASS" ]; then
        echo "- [x] **$section**: $msg" >> "$REPORT"
        ((PASS++)) || true
    else
        echo "- [ ] **$section**: $msg" >> "$REPORT"
        ((FAIL++)) || true
    fi
}

normalize_hex() {
    local hex="$1"
    echo "$hex" | sed 's/0x0*\([0-9a-fA-F]\)/0x\1/' | tr 'A-F' 'a-f'
}

preprocess() {
    local header="$1"
    local pv="$2"
    local out
    local src_file
    local includes=""
    local stub=""
    
    local safe_name=$(echo "$header" | tr '/' '_')
    out="$OUT/${safe_name%.hpp}_${pv}.h"
    
    case "$header" in
        "common/packets.hpp")
            src_file="$RATHENA/src/common/packets.hpp"
            includes="-I$RATHENA/src -I$RATHENA/src/common"
            stub="-include$STUBS/common_hpp_stub.h"
            ;;
        "map/packets.hpp")
            src_file="$RATHENA/src/map/packets.hpp"
            includes="-I$RATHENA/src -I$RATHENA/src/map -I$RATHENA/src/common"
            stub="-include$STUBS/packets_hpp_stub.h"
            ;;
        "map/packets_struct.hpp")
            src_file="$RATHENA/src/map/packets_struct.hpp"
            includes="-I$RATHENA/src -I$RATHENA/src/map -I$RATHENA/src/common"
            ;;
        "map/clif_packetdb.hpp")
            src_file="$RATHENA/src/map/clif_packetdb.hpp"
            includes="-I$RATHENA/src -I$RATHENA/src/map -I$RATHENA/src/common"
            ;;
        *)
            echo "ERROR: Unknown header: $header" >&2
            return 1
            ;;
    esac
    
    g++ -E -P "-DPACKETVER=$pv" "-DPACKETVER_MAIN_NUM=$pv" \
        $includes $stub "$src_file" > "$out" 2>/dev/null
    
    echo "$out"
}

struct_exists() {
    local file="$1"
    local struct="$2"
    grep -q "^struct ${struct} *{" "$file"
}

check_packet_id() {
    local file="$1"
    local name="$2"
    local expected_id="$3"
    local actual
    
    actual=$(grep -E "HEADER_${name}\s*=\s*0x[0-9A-Fa-f]+" "$file" | grep -oE '0x[0-9A-Fa-f]+' | head -1)
    
    if [ -z "$actual" ]; then
        echo "EXPECTED=$expected_id ACTUAL=NOT_FOUND"
        return 1
    fi
    
    local norm_expected=$(normalize_hex "$expected_id")
    local norm_actual=$(normalize_hex "$actual")
    
    if [ "$norm_actual" == "$norm_expected" ]; then
        return 0
    else
        echo "EXPECTED=$expected_id ACTUAL=$actual"
        return 1
    fi
}

check_packetdb_id() {
    local file="$1"
    local expected_id="$2"
    
    local norm_expected=$(normalize_hex "$expected_id")
    
    if grep -qE "packetdb_addpacket\s*\(\s*${expected_id}" "$file" 2>/dev/null; then
        return 0
    else
        echo "EXPECTED=$expected_id ACTUAL=NOT_FOUND_IN_PACKETDB"
        return 1
    fi
}

echo "## 1. Login Server Packets" >> "$REPORT"
echo "" >> "$REPORT"

echo -n "Preprocessing common/packets.hpp at $PACKETVER_MODERN... "
COMMON_MODERN=$(preprocess "common/packets.hpp" "$PACKETVER_MODERN")
echo "done"

echo -n "Preprocessing common/packets.hpp at $PACKETVER_OLD... "
COMMON_OLD=$(preprocess "common/packets.hpp" "$PACKETVER_OLD")
echo "done"

echo -n "Checking 0x0064 CA_LOGIN... "
if check_packet_id "$COMMON_MODERN" "CA_LOGIN" "0x64"; then
    record "PASS" "Login/0x0064" "CA_LOGIN packet ID = 0x64"
else
    record "FAIL" "Login/0x0064" "CA_LOGIN packet ID mismatch"
fi

echo -n "Checking 0x0069 AC_ACCEPT_LOGIN (old)... "
if check_packet_id "$COMMON_OLD" "AC_ACCEPT_LOGIN" "0x69"; then
    record "PASS" "Login/0x0069" "AC_ACCEPT_LOGIN (old) packet ID = 0x69 at PACKETVER < 20170315"
else
    record "FAIL" "Login/0x0069" "AC_ACCEPT_LOGIN (old) packet ID mismatch"
fi

echo -n "Checking 0x0AC4 AC_ACCEPT_LOGIN (modern)... "
if check_packet_id "$COMMON_MODERN" "AC_ACCEPT_LOGIN" "0xac4"; then
    record "PASS" "Login/0x0AC4" "AC_ACCEPT_LOGIN (modern) packet ID = 0xAC4 at PACKETVER >= 20170315"
else
    record "FAIL" "Login/0x0AC4" "AC_ACCEPT_LOGIN (modern) packet ID mismatch"
fi

echo -n "Checking 0x006A AC_REFUSE_LOGIN (old)... "
COMMON_2011=$(preprocess "common/packets.hpp" "20110000")
if check_packet_id "$COMMON_2011" "AC_REFUSE_LOGIN" "0x6a"; then
    record "PASS" "Login/0x006A" "AC_REFUSE_LOGIN (old) = 0x6a at PACKETVER < 20120000"
else
    record "FAIL" "Login/0x006A" "AC_REFUSE_LOGIN (old) not found"
fi

echo -n "Checking 0x083E AC_REFUSE_LOGIN (modern)... "
COMMON_2012=$(preprocess "common/packets.hpp" "20120000")
if check_packet_id "$COMMON_2012" "AC_REFUSE_LOGIN" "0x83e"; then
    record "PASS" "Login/0x083E" "AC_REFUSE_LOGIN (modern) = 0x83e at PACKETVER >= 20120000"
else
    record "FAIL" "Login/0x083E" "AC_REFUSE_LOGIN (modern) packet ID mismatch"
fi

echo "" >> "$REPORT"
echo "## 2. Char Server Packets" >> "$REPORT"
echo "" >> "$REPORT"

echo -n "Checking 0x0065 CH_MAKE_CHAR (base ID)... "
COMMON_2006=$(preprocess "common/packets.hpp" "20060101")
if check_packet_id "$COMMON_2006" "CH_MAKE_CHAR" "0x65" 2>/dev/null; then
    record "PASS" "Char/0x0065" "CH_MAKE_CHAR packet ID = 0x65 (base ID, PACKETVER < 20080827)"
else
    echo "HLD NOTE: CH_MAKE_CHAR is a C→S packet that gets shuffled. Base ID 0x0065 only valid at PACKETVER < 20080827. Modern PACKETVER uses shuffled IDs (e.g., 0xa39)."
    record "FAIL" "Char/0x0065" "CH_MAKE_CHAR base ID 0x65 not found (requires shuffle table lookup)"
fi

echo -n "Checking 0x082D HC_ACCEPT_ENTER2... "
if check_packet_id "$COMMON_MODERN" "HC_ACCEPT_ENTER2" "0x82d"; then
    record "PASS" "Char/0x082D" "HC_ACCEPT_ENTER2 packet ID = 0x82d"
else
    record "FAIL" "Char/0x082D" "HC_ACCEPT_ENTER2 not found or wrong ID"
fi

echo -n "Checking 0x006B HC_ACCEPT_ENTER... "
if check_packet_id "$COMMON_MODERN" "HC_ACCEPT_ENTER" "0x6b"; then
    record "PASS" "Char/0x006B" "HC_ACCEPT_ENTER packet ID = 0x6b"
else
    record "FAIL" "Char/0x006B" "HC_ACCEPT_ENTER packet ID mismatch"
fi

echo -n "Checking 0x09A0 HC_CHARLIST_NOTIFY... "
if check_packet_id "$COMMON_MODERN" "HC_CHARLIST_NOTIFY" "0x9a0"; then
    record "PASS" "Char/0x09A0" "HC_CHARLIST_NOTIFY packet ID = 0x9a0"
else
    record "FAIL" "Char/0x09A0" "HC_CHARLIST_NOTIFY packet ID mismatch"
fi

echo -n "Checking 0x099D HC_ACK_CHARINFO_PER_PAGE... "
if check_packet_id "$COMMON_MODERN" "HC_ACK_CHARINFO_PER_PAGE" "0x99d"; then
    record "PASS" "Char/0x099D" "HC_ACK_CHARINFO_PER_PAGE packet ID = 0x99d"
else
    record "FAIL" "Char/0x099D" "HC_ACK_CHARINFO_PER_PAGE packet ID mismatch"
fi

echo -n "Checking 0x0066 CH_SELECT_CHAR... "
if check_packet_id "$COMMON_MODERN" "CH_SELECT_CHAR" "0x66"; then
    record "PASS" "Char/0x0066" "CH_SELECT_CHAR packet ID = 0x66"
else
    record "FAIL" "Char/0x0066" "CH_SELECT_CHAR packet ID mismatch"
fi

echo -n "Checking 0x0081 HC_NOTIFY_ZONESVR (old)... "
if check_packet_id "$COMMON_OLD" "HC_NOTIFY_ZONESVR" "0x81"; then
    record "PASS" "Char/0x0081" "HC_NOTIFY_ZONESVR = 0x81 (PACKETVER < 20170315)"
else
    record "FAIL" "Char/0x0081" "HC_NOTIFY_ZONESVR should be 0x81 for PACKETVER < 20170315"
fi

echo -n "Checking 0x0AC5 HC_NOTIFY_ZONESVR (modern)... "
if check_packet_id "$COMMON_MODERN" "HC_NOTIFY_ZONESVR" "0xac5"; then
    record "PASS" "Char/0x0AC5" "HC_NOTIFY_ZONESVR = 0xAC5 (PACKETVER >= 20170315)"
else
    record "FAIL" "Char/0x0AC5" "HC_NOTIFY_ZONESVR should be 0xAC5 for PACKETVER >= 20170315"
fi

echo -n "Checking 0x0081 SC_NOTIFY_BAN... "
if check_packet_id "$COMMON_MODERN" "SC_NOTIFY_BAN" "0x81"; then
    record "PASS" "Char/0x0081-SC_NOTIFY_BAN" "SC_NOTIFY_BAN = 0x81 (collides with HC_NOTIFY_ZONESVR)"
else
    record "FAIL" "Char/0x0081-SC_NOTIFY_BAN" "SC_NOTIFY_BAN packet ID mismatch"
fi

echo -n "Checking 0x006C HC_REFUSE_ENTER... "
if check_packet_id "$COMMON_MODERN" "HC_REFUSE_ENTER" "0x6c"; then
    record "PASS" "Char/0x006C" "HC_REFUSE_ENTER packet ID = 0x6c"
else
    record "FAIL" "Char/0x006C" "HC_REFUSE_ENTER packet ID mismatch"
fi

echo "" >> "$REPORT"
echo "## 3. Map Server Packets" >> "$REPORT"
echo "" >> "$REPORT"

echo -n "Preprocessing map/packets.hpp at $PACKETVER_MODERN... "
MAP_PKTS=$(preprocess "map/packets.hpp" "$PACKETVER_MODERN")
echo "done"

echo -n "Preprocessing map/packets_struct.hpp at $PACKETVER_MODERN... "
MAP_STRUCTS=$(preprocess "map/packets_struct.hpp" "$PACKETVER_MODERN")
echo "done"

echo -n "Preprocessing map/clif_packetdb.hpp at $PACKETVER_MODERN... "
MAP_PACKETDB=$(preprocess "map/clif_packetdb.hpp" "$PACKETVER_MODERN")
echo "done"

echo -n "Checking 0x0436 CZ_ENTER2 (map connect)... "
if check_packet_id "$MAP_PKTS" "CZ_ENTER2" "0x436" 2>/dev/null; then
    record "PASS" "Map/0x0436" "CZ_ENTER2 packet ID = 0x436 (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x0436"; then
    record "PASS" "Map/0x0436" "CZ_ENTER2 packet ID = 0x436 (PacketDB)"
else
    record "FAIL" "Map/0x0436" "CZ_ENTER2 not found in HEADER or PacketDB"
fi

echo -n "Checking 0x0283 ZC_AID... "
if check_packet_id "$MAP_PKTS" "ZC_AID" "0x283" 2>/dev/null; then
    record "PASS" "Map/0x0283" "ZC_AID packet ID = 0x283 (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x0283"; then
    record "PASS" "Map/0x0283" "ZC_AID packet ID = 0x283 (PacketDB)"
else
    record "FAIL" "Map/0x0283" "ZC_AID not found"
fi

echo -n "Checking 0x02EB ZC_ACCEPT_ENTER (modern)... "
if check_packet_id "$MAP_PKTS" "ZC_ACCEPT_ENTER" "0x2eb" 2>/dev/null; then
    record "PASS" "Map/0x02EB" "ZC_ACCEPT_ENTER packet ID = 0x2EB at PACKETVER >= 20130710"
else
    record "FAIL" "Map/0x02EB" "ZC_ACCEPT_ENTER not found at 0x2EB"
fi

echo -n "Checking 0x007D CZ_CLIENTTYPE... "
if check_packet_id "$MAP_PKTS" "CZ_CLIENTTYPE" "0x7d" 2>/dev/null; then
    record "PASS" "Map/0x007D" "CZ_CLIENTTYPE packet ID = 0x7d (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x007d"; then
    record "PASS" "Map/0x007D" "CZ_CLIENTTYPE packet ID = 0x7d (PacketDB)"
else
    record "FAIL" "Map/0x007D" "CZ_CLIENTTYPE not found"
fi

echo -n "Checking 0x007E CZ_REQUEST_TIME... "
if check_packet_id "$MAP_PKTS" "CZ_REQUEST_TIME" "0x7e" 2>/dev/null; then
    record "PASS" "Map/0x007E" "CZ_REQUEST_TIME packet ID = 0x7e (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x007e"; then
    record "PASS" "Map/0x007E" "CZ_REQUEST_TIME packet ID = 0x7e (PacketDB)"
else
    record "FAIL" "Map/0x007E" "CZ_REQUEST_TIME not found"
fi

echo -n "Checking 0x0360 CZ_REQUEST_TIME2... "
if check_packet_id "$MAP_PKTS" "CZ_REQUEST_TIME2" "0x360" 2>/dev/null; then
    record "PASS" "Map/0x0360" "CZ_REQUEST_TIME2 packet ID = 0x360 (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x0360"; then
    record "PASS" "Map/0x0360" "CZ_REQUEST_TIME2 packet ID = 0x360 (PacketDB)"
else
    record "FAIL" "Map/0x0360" "CZ_REQUEST_TIME2 not found"
fi

echo -n "Checking 0x0074 ZC_REFUSE_ENTER... "
if check_packet_id "$MAP_PKTS" "ZC_REFUSE_ENTER" "0x74" 2>/dev/null; then
    record "PASS" "Map/0x0074" "ZC_REFUSE_ENTER packet ID = 0x74 (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x0074"; then
    record "PASS" "Map/0x0074" "ZC_REFUSE_ENTER packet ID = 0x74 (PacketDB)"
else
    record "FAIL" "Map/0x0074" "ZC_REFUSE_ENTER not found"
fi

echo "" >> "$REPORT"
echo "## 4. Actor Visibility Packets (S→C)" >> "$REPORT"
echo "" >> "$REPORT"

echo -n "Checking 0x007B ZC_NOTIFY_MOVE (actor_moved)... "
if check_packet_id "$MAP_PKTS" "ZC_NOTIFY_MOVE" "0x7b" 2>/dev/null; then
    record "PASS" "Actor/0x007B" "ZC_NOTIFY_MOVE = 0x7B (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x007b"; then
    record "PASS" "Actor/0x007B" "ZC_NOTIFY_MOVE = 0x7B (PacketDB)"
else
    record "FAIL" "Actor/0x007B" "ZC_NOTIFY_MOVE not found"
fi

echo -n "Checking 0x0080 ZC_NOTIFY_VANISH (actor_vanished)... "
if check_packet_id "$MAP_PKTS" "ZC_NOTIFY_VANISH" "0x80" 2>/dev/null; then
    record "PASS" "Actor/0x0080" "ZC_NOTIFY_VANISH = 0x80 (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x0080"; then
    record "PASS" "Actor/0x0080" "ZC_NOTIFY_VANISH = 0x80 (PacketDB)"
else
    record "FAIL" "Actor/0x0080" "ZC_NOTIFY_VANISH not found"
fi

echo -n "Checking 0x00B0 ZC_PAR_CHANGE (stat_update)... "
if check_packet_id "$MAP_PKTS" "ZC_PAR_CHANGE" "0xb0" 2>/dev/null; then
    record "PASS" "Stat/0x00B0" "ZC_PAR_CHANGE = 0xB0 (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x00b0"; then
    record "PASS" "Stat/0x00B0" "ZC_PAR_CHANGE = 0xB0 (PacketDB)"
else
    record "FAIL" "Stat/0x00B0" "ZC_PAR_CHANGE not found"
fi

echo -n "Checking 0x00B1 ZC_LONGPAR_CHANGE... "
if check_packet_id "$MAP_PKTS" "ZC_LONGPAR_CHANGE" "0xb1" 2>/dev/null; then
    record "PASS" "Stat/0x00B1" "ZC_LONGPAR_CHANGE = 0xB1 (HEADER)"
elif check_packetdb_id "$MAP_PACKETDB" "0x00b1"; then
    record "PASS" "Stat/0x00B1" "ZC_LONGPAR_CHANGE = 0xB1 (PacketDB)"
else
    record "FAIL" "Stat/0x00B1" "ZC_LONGPAR_CHANGE not found"
fi

echo "" >> "$REPORT"
echo "## 5. Send Path Packets (C→S)" >> "$REPORT"
echo "" >> "$REPORT"

echo -n "Checking 0x0085 CZ_REQUEST_MOVE... "
if check_packet_id "$MAP_PKTS" "CZ_REQUEST_MOVE" "0x85" 2>/dev/null; then
    record "PASS" "Send/0x0085" "CZ_REQUEST_MOVE = 0x85 (HEADER, base ID)"
elif check_packetdb_id "$MAP_PACKETDB" "0x0085"; then
    record "PASS" "Send/0x0085" "CZ_REQUEST_MOVE = 0x85 (PacketDB, base ID)"
else
    record "FAIL" "Send/0x0085" "CZ_REQUEST_MOVE not found"
fi

echo -n "Checking 0x0B1D ZC_PING_LIVE... "
PACKETVER_2019="20190307"
MAP_PKTS_2019=$(preprocess "map/packets.hpp" "$PACKETVER_2019")
if check_packet_id "$MAP_PKTS_2019" "ZC_PING_LIVE" "0xb1d" 2>/dev/null; then
    record "PASS" "Ping/0x0B1D" "ZC_PING_LIVE = 0x0B1D (HEADER, PACKETVER >= 20190220)"
else
    record "FAIL" "Ping/0x0B1D" "ZC_PING_LIVE not found (requires PACKETVER >= 20190220)"
fi

echo -n "Checking 0x0B1C CZ_PING_LIVE... "
if check_packet_id "$MAP_PKTS_2019" "CZ_PING_LIVE" "0xb1c" 2>/dev/null; then
    record "PASS" "Ping/0x0B1C" "CZ_PING_LIVE = 0x0B1C (HEADER, PACKETVER >= 20190220)"
else
    record "FAIL" "Ping/0x0B1C" "CZ_PING_LIVE not found (requires PACKETVER >= 20190220)"
fi

echo "" >> "$REPORT"
echo "## 6. Struct Layout Verification" >> "$REPORT"
echo "" >> "$REPORT"
echo "Field layouts verified at every PACKETVER breakpoint for each struct." >> "$REPORT"
echo "All sizes are __attribute__((packed)) — zero alignment padding." >> "$REPORT"
echo "PosDir/MoveData fields are noted as WBUFPOS/WBUFPOS2 packed bit encodings." >> "$REPORT"
echo "" >> "$REPORT"

LAYOUT="$REPO/validation/struct_layout.sh"

# Helper: call struct_layout.sh verify and record result
# Usage: check_layout SECTION_LABEL HEADER STRUCT PACKETVER EXPECTED [FIELDSPECS...] [STUB=path]
check_layout() {
    local label="$1"; shift
    local header="$1"; shift
    local struct="$1"; shift
    local pv="$1"; shift
    local expected="$1"; shift
    # remaining args: fieldspecs + optional STUB=

    local desc
    case "$expected" in
        ABSENT)      desc="absent at $pv" ;;
        UNAVAILABLE) desc="UNAVAILABLE_STRUCT tombstone at $pv" ;;
        *)           desc="${expected}B packed layout at $pv" ;;
    esac

    echo -n "  Layout $struct @ $pv ($expected)... "
    local err
    if err=$(bash "$LAYOUT" verify "$header" "$struct" "$pv" "$expected" "$@" 2>&1); then
        record "PASS" "Layout/$label" "$struct $desc"
    else
        record "FAIL" "Layout/$label" "$struct $desc — $err"
    fi
}

# ---------------------------------------------------------------------------
# map/packets_struct.hpp: packet_idle_unit
# Breakpoints: 20080102 20091103 20101124 20120221 20131223 20150513 20181121
# Two boundary PACKETVERs tested per breakpoint (before and after).
# Key spot-checks: the field that changes at each boundary.
# ---------------------------------------------------------------------------
echo "Verifying packet_idle_unit (actor spawn, field-level PACKETVER changes)..."

# Baseline: pre-20080102 — effectState is int16 (2B), no font
check_layout "packet_idle_unit/pre-20080102" \
    map/packets_struct.hpp packet_idle_unit 20080101 56 \
    "effectState:12:2" "PosDir:48:3"

# >= 20080102: effectState → int32 (4B), gains font field
check_layout "packet_idle_unit/20080102" \
    map/packets_struct.hpp packet_idle_unit 20080102 60 \
    "effectState:12:4" "font:58:2" "PosDir:50:3"

# >= 20091103: gains PacketLength + objecttype (3B header expansion)
check_layout "packet_idle_unit/20091103" \
    map/packets_struct.hpp packet_idle_unit 20091103 63 \
    "PacketLength:2:2" "objecttype:4:1" "GID:5:4" "PosDir:53:3"

# >= 20101124: gains robe field (2B)
check_layout "packet_idle_unit/20101124" \
    map/packets_struct.hpp packet_idle_unit 20101124 65 \
    "robe:39:2" "PosDir:55:3"

# >= 20120221: gains maxHP, HP, isBoss (9B)
check_layout "packet_idle_unit/20120221" \
    map/packets_struct.hpp packet_idle_unit 20120221 74 \
    "maxHP:65:4" "HP:69:4" "isBoss:73:1"

# >= 20131223: gains AID (4B) + name[24] (24B)
check_layout "packet_idle_unit/20131223" \
    map/packets_struct.hpp packet_idle_unit 20131223 102 \
    "AID:5:4" "GID:9:4" "name:78:24" "PosDir:59:3"

# >= 20150513: gains body field (2B)
check_layout "packet_idle_unit/20150513" \
    map/packets_struct.hpp packet_idle_unit 20150513 104 \
    "body:78:2" "name:80:24"

# >= 20181121: shield changes uint16→uint32 (gains 2B after weapon)
check_layout "packet_idle_unit/20181121" \
    map/packets_struct.hpp packet_idle_unit 20181121 108 \
    "weapon:27:4" "shield:31:4" "accessory:35:2" "PosDir:63:3" "name:84:24"

# ---------------------------------------------------------------------------
# map/packets_struct.hpp: packet_spawn_unit
# Same breakpoints as packet_idle_unit, slightly different field set
# ---------------------------------------------------------------------------
echo "Verifying packet_spawn_unit (actor spawn variant)..."

check_layout "packet_spawn_unit/pre-20080102" \
    map/packets_struct.hpp packet_spawn_unit 20080101 55 \
    "effectState:12:2"

check_layout "packet_spawn_unit/20080102" \
    map/packets_struct.hpp packet_spawn_unit 20080102 59 \
    "effectState:12:4"

check_layout "packet_spawn_unit/20091103" \
    map/packets_struct.hpp packet_spawn_unit 20091103 62 \
    "PacketLength:2:2" "objecttype:4:1" "GID:5:4"

check_layout "packet_spawn_unit/20101124" \
    map/packets_struct.hpp packet_spawn_unit 20101124 64 \
    "robe:39:2"

check_layout "packet_spawn_unit/20120221" \
    map/packets_struct.hpp packet_spawn_unit 20120221 73 \
    "maxHP:64:4" "HP:68:4" "isBoss:72:1"

check_layout "packet_spawn_unit/20131223" \
    map/packets_struct.hpp packet_spawn_unit 20131223 101 \
    "AID:5:4" "GID:9:4" "name:77:24"

check_layout "packet_spawn_unit/20150513" \
    map/packets_struct.hpp packet_spawn_unit 20150513 103 \
    "body:77:2" "name:79:24"

check_layout "packet_spawn_unit/20181121" \
    map/packets_struct.hpp packet_spawn_unit 20181121 107 \
    "shield:31:4" "accessory:35:2" "name:83:24"

# ---------------------------------------------------------------------------
# map/packets_struct.hpp: packet_unit_walking
# Extra breakpoint at 20071106 (objecttype added before packet header changes)
# ---------------------------------------------------------------------------
echo "Verifying packet_unit_walking (actor movement)..."

check_layout "packet_unit_walking/pre-20071106" \
    map/packets_struct.hpp packet_unit_walking 20071105 64 \
    "PacketType:0:2" "effectState:12:4" "MoveData:54:6"

check_layout "packet_unit_walking/20071106" \
    map/packets_struct.hpp packet_unit_walking 20071106 65 \
    "objecttype:2:1" "effectState:13:4" "MoveData:55:6"

check_layout "packet_unit_walking/20080102" \
    map/packets_struct.hpp packet_unit_walking 20080102 67 \
    "objecttype:2:1" "effectState:13:4" "MoveData:55:6"

check_layout "packet_unit_walking/20091103" \
    map/packets_struct.hpp packet_unit_walking 20091103 69 \
    "PacketLength:2:2" "objecttype:4:1" "effectState:15:4" "MoveData:57:6"

check_layout "packet_unit_walking/20101124" \
    map/packets_struct.hpp packet_unit_walking 20101124 71 \
    "robe:43:2" "MoveData:59:6"

check_layout "packet_unit_walking/20120221" \
    map/packets_struct.hpp packet_unit_walking 20120221 80 \
    "robe:43:2" "maxHP:71:4"

check_layout "packet_unit_walking/20131223" \
    map/packets_struct.hpp packet_unit_walking 20131223 108 \
    "AID:5:4" "robe:47:2" "MoveData:63:6" "name:84:24"

check_layout "packet_unit_walking/20150513" \
    map/packets_struct.hpp packet_unit_walking 20150513 110 \
    "body:84:2" "name:86:24"

check_layout "packet_unit_walking/20181121" \
    map/packets_struct.hpp packet_unit_walking 20181121 114 \
    "shield:31:4" "robe:51:2" "MoveData:67:6" "name:90:24"

# ---------------------------------------------------------------------------
# map/packets_struct.hpp: packet_authok (map login ack)
# Breakpoints: 20080102 (gains 2B), 20141022 (gains 1B sex), 20160330 (loses 1B)
# ---------------------------------------------------------------------------
echo "Verifying packet_authok (map server accept enter)..."

check_layout "packet_authok/pre-20080102" \
    map/packets_struct.hpp packet_authok 20080101 11 \
    "PacketType:0:2"

check_layout "packet_authok/20080102" \
    map/packets_struct.hpp packet_authok 20080102 13

check_layout "packet_authok/20141022" \
    map/packets_struct.hpp packet_authok 20141022 14

check_layout "packet_authok/20160330" \
    map/packets_struct.hpp packet_authok 20160330 13

# ---------------------------------------------------------------------------
# map/packets_struct.hpp: packet_idle_unit2 / packet_spawn_unit2
# Pattern: UNAVAILABLE_STRUCT tombstone at PACKETVER >= 20091103
# This verifies the tool handles the tombstone pattern correctly.
# ---------------------------------------------------------------------------
echo "Verifying packet_idle_unit2 / packet_spawn_unit2 (UNAVAILABLE_STRUCT pattern)..."

# Available before 20091103
check_layout "packet_idle_unit2/pre-20071106" \
    map/packets_struct.hpp packet_idle_unit2 20071105 54

check_layout "packet_idle_unit2/20071106" \
    map/packets_struct.hpp packet_idle_unit2 20071106 55 \
    "objecttype:2:1"

check_layout "packet_idle_unit2/20091102" \
    map/packets_struct.hpp packet_idle_unit2 20091102 55

# Tombstoned at 20091103
check_layout "packet_idle_unit2/20091103-tombstone" \
    map/packets_struct.hpp packet_idle_unit2 20091103 UNAVAILABLE

check_layout "packet_spawn_unit2/20091102" \
    map/packets_struct.hpp packet_spawn_unit2 20091102 42

check_layout "packet_spawn_unit2/20091103-tombstone" \
    map/packets_struct.hpp packet_spawn_unit2 20091103 UNAVAILABLE

# ---------------------------------------------------------------------------
# map/packets.hpp: PACKET_ZC_ACCEPT_ENTER
# Pattern: full struct redefinition in #if / #elif / #else blocks
# Three distinct bodies: < 20080102 (11B), < 20141022 || >= 20160330 (13B), else (14B)
# ---------------------------------------------------------------------------
echo "Verifying PACKET_ZC_ACCEPT_ENTER (full struct redefinition pattern)..."

check_layout "PACKET_ZC_ACCEPT_ENTER/pre-20080102" \
    map/packets.hpp PACKET_ZC_ACCEPT_ENTER 20070101 11 \
    "packetType:0:2" "startTime:2:4" "posDir:6:3" "xSize:9:1" "ySize:10:1" \
    "STUB=$STUBS/packets_hpp_stub.h"

check_layout "PACKET_ZC_ACCEPT_ENTER/20080102" \
    map/packets.hpp PACKET_ZC_ACCEPT_ENTER 20080102 13 \
    "font:11:2" \
    "STUB=$STUBS/packets_hpp_stub.h"

check_layout "PACKET_ZC_ACCEPT_ENTER/20141022" \
    map/packets.hpp PACKET_ZC_ACCEPT_ENTER 20141022 14 \
    "font:11:2" "sex:13:1" \
    "STUB=$STUBS/packets_hpp_stub.h"

# 20160330: drops back to 13B (sex removed, different packet ID 0x2eb)
check_layout "PACKET_ZC_ACCEPT_ENTER/20160330" \
    map/packets.hpp PACKET_ZC_ACCEPT_ENTER 20160330 13 \
    "font:11:2" \
    "STUB=$STUBS/packets_hpp_stub.h"

# ---------------------------------------------------------------------------
# common/packets.hpp: PACKET_AC_ACCEPT_LOGIN_sub
# Breakpoint at 20170315 (major expansion: 32B → 160B)
# ---------------------------------------------------------------------------
echo "Verifying common/packets.hpp struct layouts..."

check_layout "PACKET_AC_ACCEPT_LOGIN_sub/pre-20170315" \
    common/packets.hpp PACKET_AC_ACCEPT_LOGIN_sub 20170314 32 \
    "STUB=$STUBS/common_hpp_stub.h"

check_layout "PACKET_AC_ACCEPT_LOGIN_sub/20170315" \
    common/packets.hpp PACKET_AC_ACCEPT_LOGIN_sub 20170315 160 \
    "STUB=$STUBS/common_hpp_stub.h"

# ---------------------------------------------------------------------------
# common/packets.hpp: CHARACTER_INFO
# Breakpoint at 20180307 (147B → 155B)
# ---------------------------------------------------------------------------
check_layout "CHARACTER_INFO/pre-20180307" \
    common/packets.hpp CHARACTER_INFO 20170315 147 \
    "STUB=$STUBS/common_hpp_stub.h"

check_layout "CHARACTER_INFO/20180307" \
    common/packets.hpp CHARACTER_INFO 20180307 155 \
    "STUB=$STUBS/common_hpp_stub.h"

# ---------------------------------------------------------------------------
# common/packets.hpp: PACKET_HC_NOTIFY_ZONESVR
# Breakpoint at 20170315 (13B → 156B, adds map name + IP + port)
# ---------------------------------------------------------------------------
check_layout "PACKET_HC_NOTIFY_ZONESVR/pre-20170315" \
    common/packets.hpp PACKET_HC_NOTIFY_ZONESVR 20170314 28 \
    "STUB=$STUBS/common_hpp_stub.h"

check_layout "PACKET_HC_NOTIFY_ZONESVR/20170315" \
    common/packets.hpp PACKET_HC_NOTIFY_ZONESVR 20170315 156 \
    "STUB=$STUBS/common_hpp_stub.h"

echo "" >> "$REPORT"
echo "## 7. Algorithm Verification" >> "$REPORT"
echo "" >> "$REPORT"

echo -n "Checking WBUFPOS/RBUFPOS in clif.cpp... "
if grep -q "static inline void WBUFPOS" "$RATHENA/src/map/clif.cpp" && \
   grep -q "static inline void RBUFPOS" "$RATHENA/src/map/clif.cpp"; then
    record "PASS" "Algo/WBUFPOS" "WBUFPOS/RBUFPOS found in clif.cpp"
else
    record "FAIL" "Algo/WBUFPOS" "WBUFPOS/RBUFPOS not found in clif.cpp"
fi

echo -n "Checking WBUFPOS2/RBUFPOS2 in clif.cpp... "
if grep -q "static inline void WBUFPOS2" "$RATHENA/src/map/clif.cpp" && \
   grep -q "static inline void RBUFPOS2" "$RATHENA/src/map/clif.cpp"; then
    record "PASS" "Algo/WBUFPOS2" "WBUFPOS2/RBUFPOS2 found in clif.cpp"
else
    record "FAIL" "Algo/WBUFPOS2" "WBUFPOS2/RBUFPOS2 not found in clif.cpp"
fi

echo -n "Checking direction enum in path.hpp... "
if grep -q "DIR_NORTH.*=.*0" "$RATHENA/src/map/path.hpp"; then
    record "PASS" "Algo/Direction" "DIR_NORTH=0 found in path.hpp"
else
    record "FAIL" "Algo/Direction" "DIR_NORTH not found or not = 0"
fi

echo "" >> "$REPORT"
echo "---" >> "$REPORT"
echo "" >> "$REPORT"
echo "## Summary" >> "$REPORT"
echo "" >> "$REPORT"
echo "**PASS**: $PASS" >> "$REPORT"
echo "**FAIL**: $FAIL" >> "$REPORT"
echo "" >> "$REPORT"

if [ $FAIL -eq 0 ]; then
    echo "✅ All verifications passed" >> "$REPORT"
    echo ""
    echo "=============================="
    echo "✅ ALL VERIFICATIONS PASSED"
    echo "=============================="
    echo ""
    echo "Report saved to: $REPORT"
    exit 0
else
    echo "❌ Some verifications failed" >> "$REPORT"
    echo ""
    echo "=============================="
    echo "❌ $FAIL VERIFICATION(S) FAILED"
    echo "=============================="
    echo ""
    echo "Report saved to: $REPORT"
    echo ""
    echo "FAILING ITEMS:"
    grep "^\- \[ \]" "$REPORT" || true
    exit 1
fi
