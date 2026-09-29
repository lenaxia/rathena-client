RATHENA_REF != cat hack/testserver/RATHENA_REF
CAPTURES_DIR ?= captures
LABEL        ?= cap

.PHONY: test testserver-up testserver-down testserver-down-v testserver-logs test-integration \
        capture-up capture capture-down fuzz-nightly codegen-check

test:
	go test -count=1 ./...

# --- live rAthena test harness ----------------------------------------------

testserver-up:
	docker compose -f hack/testserver/docker-compose.yml up -d --build

testserver-down:
	docker compose -f hack/testserver/docker-compose.yml down

testserver-down-v:
	docker compose -f hack/testserver/docker-compose.yml down -v

testserver-logs:
	docker compose -f hack/testserver/docker-compose.yml logs -f rathena

test-integration:
	RATHENA_ADDR=127.0.0.1:6900 RATHENA_PACKETVER=20200401 \
	RATHENA_USER=testbot RATHENA_PASS=testpass RATHENA_CHARSLOT=0 \
	go test -tags integration -timeout 120s ./pkg/session/ -run TestLiveServer -v

# --- pktmirror capture ------------------------------------------------------

COMPOSE_CAPTURE = docker compose -f hack/testserver/docker-compose.yml \
                  -f hack/testserver/compose.pktmirror.yml

capture-up:
	$(COMPOSE_CAPTURE) up -d --build

capture-down:
	$(COMPOSE_CAPTURE) down

# Run pktmirror against the capture topology; Ctrl-C to flush the fixture.
capture:
	go run ./cmd/pktmirror -trio -packetver 20200401 -label $(LABEL) -out $(CAPTURES_DIR)

# --- fuzzing ----------------------------------------------------------------

# Seed-corpus smoke is already part of `go test ./...`; this runs short live
# bursts per target. Nightly CI runs the long version.
fuzz-smoke:
	go test ./pkg/session/ -run '^$$' -fuzz FuzzFeedMapSessionWithDispatch -fuzztime 15s
	go test ./pkg/session/ -run '^$$' -fuzz FuzzFeedLoginSession -fuzztime 15s
	go test ./pkg/session/ -run '^$$' -fuzz FuzzFeedCharSession -fuzztime 15s
	go test ./pkg/packing/ -run '^$$' -fuzz FuzzPosDirRoundTrip -fuzztime 15s
	go test ./pkg/packing/ -run '^$$' -fuzz FuzzMoveDataRoundTrip -fuzztime 15s

# --- codegen drift gate -----------------------------------------------------

# Regenerates pkg/ from the pinned rAthena ref and fails on any diff.
# Requires gcc/g++ (GCC preprocessor) and network access to clone rAthena.
codegen-check:
	rm -rf .codegen-rathena
	git clone --no-checkout https://github.com/rathena/rathena.git .codegen-rathena
	git -C .codegen-rathena checkout $(RATHENA_REF)
	go run ./internal/codegen/main.go --rathena .codegen-rathena --out .
	git diff --exit-code -- pkg semantics || (echo "FAIL: generated code is stale — commit the regeneration" && exit 1)
	rm -rf .codegen-rathena
