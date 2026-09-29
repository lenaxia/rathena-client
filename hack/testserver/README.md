# rAthena live test harness

A reproducible rAthena server (pinned ref, `PACKETVER=20200401`) for:

- `pkg/session` live integration tests (`fsm_live_integration_test.go`)
- ground-truth captures via `cmd/pktmirror` → new replay fixtures

rAthena is GPL-3.0; it is used strictly as an external test dependency built
and run inside a container. Nothing from it is linked into or distributed with
this Apache-2.0 repository. The pinned ref lives in `RATHENA_REF` and is shared
with the codegen drift CI job.

## Start / stop

    make testserver-up     # builds the image (~5-15 min first time), starts db + rathena
    make testserver-logs   # tail server logs
    make testserver-down-v  # stop and also drop the db volume

Server: login `127.0.0.1:6900`, char `127.0.0.1:6121`, map `127.0.0.1:5121`.
Test account: `testbot` / `testpass`, character `TestBot` in slot 0 (Prontera).

## Run the live integration test

    make test-integration

Equivalent to:

    RATHENA_ADDR=127.0.0.1:6900 RATHENA_PACKETVER=20200401 \
    RATHENA_USER=testbot RATHENA_PASS=testpass RATHENA_CHARSLOT=0 \
    go test -tags integration -timeout 120s ./pkg/session/ -run TestLiveServer -v

The test skips itself when the server is not reachable.

## Capturing new replay fixtures with pktmirror

`cmd/pktmirror` proxies the client↔server trio and writes `.fixture` files in
the same RATF format as `pkg/session/testdata/`.

    # 1. Start rathena with shifted published ports (36900/36121/35121) so
    #    pktmirror can own the advertised addresses 6900/6121/5121:
    make capture-up
    # 2. Run the mirror (Ctrl-C when done; it then writes the fixture):
    make capture LABEL=dump9
    # 3. Or point any client (goKore) at 127.0.0.1:6900 and play the scenario.
    # 4. Stop:
    make capture-down

Outputs land in `captures/`: per-phase hexdump logs, raw C→S / S→C byte
streams, and `<label>_<timestamp>.fixture` containing the S→C phases.

Turning a capture into a regression test:

    cp captures/<label>_*.fixture pkg/session/testdata/<name>_<packetver>.fixture

then add a case to `fsm_replay_test.go` (see `runReplayTest`) and register
assertion handlers. Missing S→C lengths discovered this way should be fixed in
codegen/mappings so the generated tables carry them.

## Files

- `Dockerfile.rathena` — multi-stage build, `--enable-packetver=20200401`
- `RATHENA_REF` — pinned rathena commit (shared with codegen drift CI)
- `entrypoint.sh` — waits for db, imports schema + seed once, runs the trio
- `conf/import/` — rAthena conf overrides (advertise 127.0.0.1, db = compose service)
- `seed/01-test-account.sql` — test account + character
- `docker-compose.yml` — default topology (direct ports)
- `compose.pktmirror.yml` — capture overlay (shifted ports)
