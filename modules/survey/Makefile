# Module-local dev loop. The root `make dev-module` builds from the sdk Go
# module and cannot build this directory (it has its own go.mod), so this
# target mirrors its environment.
.PHONY: dev test selftest raw

dev:
	mkdir -p ../../bin
	go build -o ../../bin/waypoint-module-survey ./cmd/waypoint-module-survey
	@echo "dev needs a dev rover running in another terminal: make dev-rover"
	WAYPOINT_NATS_URL=nats://127.0.0.1:4222 \
	WAYPOINT_ROVER_ID=sim-rover \
	WAYPOINT_MODULE_ID=survey \
	WAYPOINT_MODULE_CONFIG=$(CURDIR)/dev-config.toml \
	../../bin/waypoint-module-survey

test:
	go test ./...

selftest:
	go run ./cmd/waypoint-module-survey --selftest

raw:
	$(MAKE) -f build/Makefile raw
