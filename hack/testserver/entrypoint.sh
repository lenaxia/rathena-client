#!/bin/sh
# entrypoint.sh — waits for MariaDB, imports schema + seed data once, then
# runs the rAthena login/char/map trio in the foreground.
set -eu

DB_HOST="${DB_HOST:-db}"
DB_PORT="${DB_PORT:-3306}"
DB_NAME="${DB_NAME:-ragnarok}"
DB_USER="${DB_USER:-ragnarok}"
DB_PASS="${DB_PASS:-ragnarok}"
ROOT_PASS="${ROOT_PASS:-ragnarok-root}"

mysql_cli() {
    mysql -h "$DB_HOST" -P "$DB_PORT" -u root -p"$ROOT_PASS" "$@"
}

echo "[entrypoint] waiting for MariaDB at $DB_HOST:$DB_PORT ..."
i=0
until mysqladmin ping -h "$DB_HOST" -P "$DB_PORT" -u root -p"$ROOT_PASS" --silent 2>/dev/null; do
    i=$((i + 1))
    if [ "$i" -ge 60 ]; then
        echo "[entrypoint] FATAL: MariaDB not reachable after 60 attempts" >&2
        exit 1
    fi
    sleep 2
done

# Import schema + seed exactly once (marker table).
if ! mysql_cli -e "USE \`$DB_NAME\`; SELECT 1 FROM codegen_marker LIMIT 1;" >/dev/null 2>&1; then
    echo "[entrypoint] importing rAthena schema ..."
    mysql_cli -e "CREATE DATABASE IF NOT EXISTS \`$DB_NAME\`;"
    mysql_cli "$DB_NAME" < /opt/rathena/sql-files/main.sql
    echo "[entrypoint] importing seed data ..."
    if mysql_cli "$DB_NAME" < /seed/01-test-account.sql; then
        mysql_cli -e "USE \`$DB_NAME\`; CREATE TABLE codegen_marker (ok TINYINT PRIMARY KEY) ENGINE=InnoDB; INSERT INTO codegen_marker VALUES (1);"
        echo "[entrypoint] seed complete"
    else
        echo "[entrypoint] WARNING: seed import failed — login test account may be missing" >&2
    fi
else
    echo "[entrypoint] schema already imported, skipping"
fi

echo "[entrypoint] starting rAthena (login:6900 char:6121 map:5121, PACKETVER=20200401)"
cd /opt/rathena
./login-server &
LOGIN_PID=$!
./char-server &
sleep 3
# Fail fast if either background server died (e.g. DB unreachable) instead of
# limping on with a dead login server and confusing connection resets.
for pid_name in "$LOGIN_PID:login"; do
    pid="${pid_name%%:*}"
    name="${pid_name##*:}"
    if ! kill -0 "$pid" 2>/dev/null; then
        echo "[entrypoint] FATAL: $name-server exited during startup" >&2
        tail -n 40 "log/$name-server.log" >&2 2>/dev/null || true
        exit 1
    fi
done
exec ./map-server
