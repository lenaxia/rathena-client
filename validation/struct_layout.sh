#!/usr/bin/env bash
# struct_layout.sh — Extract and verify struct field layouts from rAthena headers
#
# Usage:
#   struct_layout.sh dump   HEADER STRUCT PACKETVER [STUB=path]
#   struct_layout.sh verify HEADER STRUCT PACKETVER EXPECTED_OUTCOME [FIELDSPEC...] [STUB=path]
#   struct_layout.sh status HEADER STRUCT PACKETVER [STUB=path]
#
# Commands:
#   dump    — Print full field table (name, type, offset, size) to stdout
#   verify  — Check total bytes and optional spot-check fields; exit 0=pass 1=fail
#   status  — Print one of: AVAILABLE | UNAVAILABLE | ABSENT
#
# HEADER is relative to rAthena src/, e.g.:
#   map/packets_struct.hpp
#   map/packets.hpp
#   common/packets.hpp
#
# EXPECTED_OUTCOME for verify:
#   ABSENT               — struct must not exist in preprocessed output
#   UNAVAILABLE          — struct must contain UNAVAILABLE_STRUCT sentinel
#   <integer>            — struct must have exactly this many total bytes (packed layout)
#
# FIELDSPEC format (only meaningful when EXPECTED_OUTCOME is an integer):
#   name:expected_offset:expected_size
#   e.g.  effectState:12:4   PosDir:48:3   name:78:24
#
# STUB=path is an optional -include file for preprocessor stubs.
#
# Output format for dump (tab-separated):
#   field   NAME   TYPE   OFFSET   SIZE   [NOTE]
#   total   BYTES
#   (or: ABSENT / UNAVAILABLE on stdout with exit 1)
#
# Exit codes:
#   0 = success / assertion passed
#   1 = error / assertion failed
#
set -euo pipefail

RATHENA="${RATHENA_ROOT:-$HOME/personal/rathena}"
REPO="$(cd "$(dirname "$0")/.." && pwd)"
CACHE_DIR="$REPO/validation/output/layouts"
mkdir -p "$CACHE_DIR"

# ---------------------------------------------------------------------------
# Preprocess a header at a given PACKETVER; return path to cached output file
# ---------------------------------------------------------------------------
preprocess() {
    local header="$1"
    local pv="$2"
    local stub="${3:-}"

    local safe_header
    safe_header=$(echo "$header" | tr '/' '_' | sed 's/\.hpp$//')
    local cache_file="$CACHE_DIR/${safe_header}_${pv}.h"

    if [ -f "$cache_file" ]; then
        echo "$cache_file"
        return 0
    fi

    local src_file="$RATHENA/src/$header"
    if [ ! -f "$src_file" ]; then
        echo "ERROR: source file not found: $src_file" >&2
        return 1
    fi

    local includes="-I$RATHENA/src"
    case "$header" in
        map/*)    includes="$includes -I$RATHENA/src/map -I$RATHENA/src/common" ;;
        common/*) includes="$includes -I$RATHENA/src/common" ;;
    esac

    local stub_flag=""
    [ -n "$stub" ] && stub_flag="-include$stub"

    # shellcheck disable=SC2086
    g++ -E -P \
        "-DPACKETVER=$pv" \
        "-DPACKETVER_MAIN_NUM=$pv" \
        $includes \
        $stub_flag \
        "$src_file" > "$cache_file" 2>/dev/null

    echo "$cache_file"
}

# ---------------------------------------------------------------------------
# Extract struct body from preprocessed file into a tmpfile
# Sets global STRUCT_BODY_FILE and STRUCT_STATUS:
#   ABSENT      — struct name not found
#   UNAVAILABLE — UNAVAILABLE_STRUCT sentinel present
#   AVAILABLE   — parseable field list
# ---------------------------------------------------------------------------
STRUCT_BODY_FILE=""
STRUCT_STATUS=""

extract_struct() {
    local preprocessed="$1"
    local struct="$2"

    STRUCT_BODY_FILE=$(mktemp /tmp/struct_layout_XXXXXX.txt)

    awk -v s="$struct" '
        /^struct [A-Za-z_][A-Za-z0-9_]* *\{/ {
            n = $0
            sub(/^struct /, "", n)
            sub(/ *\{.*/, "", n)
            if (n == s) { in_struct=1; next }
        }
        in_struct {
            if (/^\}/) { exit }
            print
        }
    ' "$preprocessed" > "$STRUCT_BODY_FILE"

    if [ ! -s "$STRUCT_BODY_FILE" ]; then
        STRUCT_STATUS="ABSENT"
    elif grep -q "UNAVAILABLE_STRUCT" "$STRUCT_BODY_FILE"; then
        STRUCT_STATUS="UNAVAILABLE"
    else
        STRUCT_STATUS="AVAILABLE"
    fi
}

cleanup_body() {
    [ -n "$STRUCT_BODY_FILE" ] && rm -f "$STRUCT_BODY_FILE"
    STRUCT_BODY_FILE=""
}

# ---------------------------------------------------------------------------
# Parse struct body file into field table
# Writes tab-separated lines to stdout:
#   field  NAME  TYPE  OFFSET  SIZE  NOTE
#   total  BYTES
# Caller must ensure STRUCT_STATUS == AVAILABLE before calling
# ---------------------------------------------------------------------------
PARSE_SCRIPT="$REPO/validation/parse_struct_fields.py"

parse_fields() {
    local body_file="$1"
    python3 "$PARSE_SCRIPT" "$body_file"
}

# ---------------------------------------------------------------------------
# status command
# ---------------------------------------------------------------------------
cmd_status() {
    local header="$1"
    local struct="$2"
    local pv="$3"
    local stub=""
    for arg in "${@:4}"; do [[ "$arg" == STUB=* ]] && stub="${arg#STUB=}"; done

    local preprocessed
    preprocessed=$(preprocess "$header" "$pv" "$stub")
    extract_struct "$preprocessed" "$struct"
    local status="$STRUCT_STATUS"
    cleanup_body
    echo "$status"
}

# ---------------------------------------------------------------------------
# dump command
# ---------------------------------------------------------------------------
cmd_dump() {
    local header="$1"
    local struct="$2"
    local pv="$3"
    local stub=""
    for arg in "${@:4}"; do [[ "$arg" == STUB=* ]] && stub="${arg#STUB=}"; done

    local preprocessed
    preprocessed=$(preprocess "$header" "$pv" "$stub")
    extract_struct "$preprocessed" "$struct"

    case "$STRUCT_STATUS" in
        ABSENT)
            cleanup_body
            echo "ABSENT"
            return 1
            ;;
        UNAVAILABLE)
            cleanup_body
            echo "UNAVAILABLE"
            return 1
            ;;
    esac

    parse_fields "$STRUCT_BODY_FILE"
    cleanup_body
}

# ---------------------------------------------------------------------------
# verify command
# ---------------------------------------------------------------------------
cmd_verify() {
    local header="$1"
    local struct="$2"
    local pv="$3"
    local expected="$4"
    shift 4

    local stub=""
    local fieldspecs=()
    for arg in "$@"; do
        if [[ "$arg" == STUB=* ]]; then
            stub="${arg#STUB=}"
        else
            fieldspecs+=("$arg")
        fi
    done

    local preprocessed
    preprocessed=$(preprocess "$header" "$pv" "$stub")
    extract_struct "$preprocessed" "$struct"

    # --- ABSENT ---
    if [ "$expected" = "ABSENT" ]; then
        if [ "$STRUCT_STATUS" = "ABSENT" ]; then
            cleanup_body; return 0
        else
            echo "FAIL [$struct @ $pv]: expected ABSENT but status=$STRUCT_STATUS" >&2
            cleanup_body; return 1
        fi
    fi

    # For all other checks, struct must exist
    if [ "$STRUCT_STATUS" = "ABSENT" ]; then
        echo "FAIL [$struct @ $pv]: expected $expected but struct is ABSENT" >&2
        cleanup_body; return 1
    fi

    # --- UNAVAILABLE ---
    if [ "$expected" = "UNAVAILABLE" ]; then
        if [ "$STRUCT_STATUS" = "UNAVAILABLE" ]; then
            cleanup_body; return 0
        else
            echo "FAIL [$struct @ $pv]: expected UNAVAILABLE but got AVAILABLE" >&2
            cleanup_body; return 1
        fi
    fi

    # --- AVAILABLE + size check ---
    if [ "$STRUCT_STATUS" = "UNAVAILABLE" ]; then
        echo "FAIL [$struct @ $pv]: expected total=$expected but struct is UNAVAILABLE" >&2
        cleanup_body; return 1
    fi

    local parsed
    parsed=$(parse_fields "$STRUCT_BODY_FILE")
    cleanup_body

    local actual_total
    actual_total=$(echo "$parsed" | awk -F'\t' '$1=="total" {print $2}')

    if [ "$actual_total" != "$expected" ]; then
        echo "FAIL [$struct @ $pv]: total bytes expected=$expected actual=$actual_total" >&2
        echo "$parsed" | awk -F'\t' '$1=="field" {printf "  %-22s %-14s offset=%-5s size=%s\n",$2,$3,$4,$5}' >&2
        return 1
    fi

    # --- Field spot-checks ---
    local fail=0
    for spec in "${fieldspecs[@]}"; do
        local fname foff fsize
        IFS=':' read -r fname foff fsize <<< "$spec"

        local actual_off actual_size
        actual_off=$(echo "$parsed"  | awk -F'\t' -v n="$fname" '$1=="field" && $2==n {print $4}')
        actual_size=$(echo "$parsed" | awk -F'\t' -v n="$fname" '$1=="field" && $2==n {print $5}')

        if [ -z "$actual_off" ]; then
            echo "FAIL [$struct @ $pv]: field '$fname' not found (expected offset=$foff size=$fsize)" >&2
            fail=1; continue
        fi
        if [ "$actual_off" != "$foff" ]; then
            echo "FAIL [$struct @ $pv]: field '$fname' offset expected=$foff actual=$actual_off" >&2
            fail=1
        fi
        if [ "$actual_size" != "$fsize" ]; then
            echo "FAIL [$struct @ $pv]: field '$fname' size expected=$fsize actual=$actual_size" >&2
            fail=1
        fi
    done

    return $fail
}

# ---------------------------------------------------------------------------
# Main dispatch
# ---------------------------------------------------------------------------
CMD="${1:-}"
if [ -z "$CMD" ]; then
    echo "Usage: struct_layout.sh <dump|verify|status> HEADER STRUCT PACKETVER [args...]" >&2
    exit 1
fi
shift

case "$CMD" in
    dump)   cmd_dump   "$@" ;;
    verify) cmd_verify "$@" ;;
    status) cmd_status "$@" ;;
    *)
        echo "Unknown command: $CMD" >&2
        exit 1
        ;;
esac
