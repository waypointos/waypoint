.PHONY: all sim agent dashboard proto test fmt clean foundation-smoke embed-dashboard embed-proxy-dashboard proxy proxy-test proxy-init-operator proxy-smoke fleet-smoke cameras-smoke auth-smoke release-image release-proxy web-stub

all: proto
	@echo "Run 'make sim' / 'make agent' / 'make dashboard' in separate terminals."

proto:
	cd protocol && buf generate

sim:
	$(MAKE) -C sim run

agent:
	$(MAKE) -C agent run

dashboard:
	cd dashboard && pnpm dev

# agent/web and proxy/web go:embed dist/, which only exists after a
# dashboard build; give fresh checkouts a stub so Go code compiles. The
# real build (embed-dashboard / embed-proxy-dashboard) replaces it.
web-stub:
	@for d in agent/web/dist proxy/web/dist; do \
	  mkdir -p $$d; \
	  [ -f $$d/index.html ] || printf '<!doctype html><title>dashboard not embedded; run make embed-dashboard</title>\n' > $$d/index.html; \
	done

test: web-stub
	cd agent && go test ./...
	cd sim && go test ./...
	cd sdk && go test ./...
	cd protocol/platform && go test ./...
	cd dashboard && pnpm test
	$(MAKE) -C proxy test
	$(MAKE) firmware-test

fmt:
	cd agent && gofmt -w .
	cd sim && gofmt -w .
	cd dashboard && pnpm fmt

clean:
	rm -rf agent/web/dist dashboard/dist

foundation-smoke:
	@echo "Open three terminals:"
	@echo "  T1: make agent      # starts embedded NATS on :4222 and HTTP gateway on :8080"
	@echo "  T2: make sim        # connects to NATS at 127.0.0.1:4222, publishes telemetry, listens for cmd.drive"
	@echo "  T3: make dashboard  # Vite dev server on :5173, proxies /ws to agent:8080"
	@echo ""
	@echo "Then open http://localhost:5173/?token=dev-token"
	@echo "  - Fleet view shows rover-dev as Online with bus voltage flickering ~12.4 V"
	@echo "  - Click into rover-dev"
	@echo "  - Drag the joystick. Motor currents and temperatures rise; FR climbs visibly faster (asymmetry in the fake bus)"
	@echo "  - Release joystick — telemetry returns to idle values"

proxy:
	$(MAKE) -C proxy run

proxy-test:
	$(MAKE) -C proxy test

proxy-init-operator:
	cd proxy && go run ./cmd/waypoint-operator-init

proxy-smoke:
	@echo "1. docker compose up -d postgres"
	@echo "2. cp proxy/.env.example proxy/.env  # then fill in values"
	@echo "3. make proxy-init-operator  # save the printed seed into proxy/.env"
	@echo "4. make proxy"
	@echo "5. visit http://localhost:8081"

fleet-smoke:
	@echo "Phase 2 end-to-end smoke. Prerequisite: proxy is deployed on Railway."
	@echo
	@echo "1. Open https://<railway-url> in a browser and sign in via WorkOS."
	@echo "   You should land on the fleet view (empty)."
	@echo "2. Click 'Add rover' → enter id='sim-01' name='Sim'. Copy the identity.toml."
	@echo "3. Save the file as sim/sim-identity.toml."
	@echo "4. Restart the sim:"
	@echo "     cd sim && go run ./cmd/waypoint-sim --identity ./sim-identity.toml"
	@echo "5. Refresh the proxy fleet view; sim-01 should appear online."
	@echo "6. Click it; telemetry should update live and the joystick should drive it."
	@echo "7. (Optional) Sign in as a second WorkOS user; the fleet should be empty for them."
	@echo "   Back as admin, visit /admin and grant the second user 'control' on sim-01."
	@echo "   Reload as the second user; sim-01 appears and is drivable."

cameras-smoke:
	@echo "Phase 3 Mac smoke. Prerequisite: brew install gstreamer + plugins."
	@echo "  See docs/setup/cameras.md for the exact incantation."
	@echo
	@echo "1. Edit agent/identity.toml to include:"
	@echo "     [[cameras]]"
	@echo "     name = \"chassis-front\""
	@echo "     pipeline = \"synthetic\""
	@echo
	@echo "2. (Proxy mode only) Start postgres + proxy:"
	@echo "     docker compose up -d postgres"
	@echo "     cd proxy && go run ./cmd/waypoint-proxy"
	@echo
	@echo "3. Start the agent: cd agent && go run ./cmd/waypoint-agent"
	@echo
	@echo "4. Direct mode: open http://localhost:5173/ui-gallery"
	@echo "   Scroll to the Camera view tile; you should see a moving test pattern."
	@echo
	@echo "5. Proxy mode: open https://<proxy-url>/, sign in, click your rover,"
	@echo "   see the same stream rendered through the WHEP-over-NATS tunnel."
	@echo
	@echo "6. Real webcam (Mac only): set pipeline = \"mac\" and restart agent."

embed-dashboard:
	cd dashboard && pnpm build
	rm -rf agent/web/dist
	mkdir -p agent/web/dist
	cp -r dashboard/dist/. agent/web/dist/
	cd agent && go build ./...

# Same as embed-dashboard but for the proxy's go:embed. Replaces the
# checked-in placeholder index.html with the real Vite build. The proxy
# must be rebuilt (make proxy / go run) afterwards to pick up the new
# embedded bytes.
embed-proxy-dashboard:
	cd dashboard && pnpm build
	rm -rf proxy/web/dist
	mkdir -p proxy/web/dist
	cp -r dashboard/dist/. proxy/web/dist/
	cd proxy && go build ./...

.PHONY: core core-test core-smoke dev-rover dev-module

core:
	cmake -S core -B core/build
	cmake --build core/build

PLATFORM ?= rover

dev-rover: ## build agent + core and run a simulated rover locally (PLATFORM=rover|bench)
	cmake -S core -B core/build -DWP_CORE_BUILD_TESTS=OFF
	cmake --build core/build -j
	mkdir -p bin && cd agent && go build -o ../bin/waypoint-agent ./cmd/waypoint-agent
	cd sim && go run ./cmd/waypoint-dev --descriptor ../protocol/platform/waypoint-$(PLATFORM).toml

dev-module: ## run a module binary against a dev rover: make dev-module MODULE=./sdk/examples/arm-sim MODULE_ID=arm-sim
	@test -n "$(MODULE)" || { echo "usage: make dev-module MODULE=<pkg dir> MODULE_ID=<id> [MODULE_ENV='K=V ...']"; exit 1; }
	@echo "dev-module needs a dev rover running in another terminal: make dev-rover"
	mkdir -p bin && cd sdk && go build -o ../bin/dev-module $(abspath $(MODULE))
	# Component class/rate come from the manifest on a real rover (the agent's
	# drop-in); the dev loop reads them from module.toml so declared rates apply.
	WAYPOINT_NATS_URL=nats://127.0.0.1:4222 \
	WAYPOINT_ROVER_ID=sim-rover \
	WAYPOINT_MODULE_ID=$(MODULE_ID) \
	WAYPOINT_MODULE_COMPONENT=$$(sed -n 's/^class *= *"\(.*\)".*/\1/p' $(MODULE)/module.toml 2>/dev/null | head -1) \
	WAYPOINT_MODULE_STATE_RATE_HZ=$$(sed -n 's/^state_rate_hz *= *\([0-9.]*\).*/\1/p' $(MODULE)/module.toml 2>/dev/null | head -1) \
	$(MODULE_ENV) ./bin/dev-module

core-test:
	# Pin tests ON: a cached -DWP_CORE_BUILD_TESTS=OFF from a binary-only build
	# would otherwise leave ctest running a stale test binary that still passes.
	cmake -S core -B core/build -DWP_CORE_BUILD_TESTS=ON
	cmake --build core/build
	ctest --test-dir core/build --output-on-failure

core-smoke:
	@echo "Phase 4 Mac smoke. Prerequisite: brew install cmake protobuf googletest"
	@echo
	@echo "1. Start agent (with Unix relay enabled):"
	@echo "     cd agent && go run ./cmd/waypoint-agent"
	@echo
	@echo "2. Confirm /tmp/waypoint-nats.sock exists:"
	@echo "     ls -la /tmp/waypoint-nats.sock"
	@echo
	@echo "3. Build + run all core unit + integration tests:"
	@echo "     make core-test"
	@echo
	@echo "4. Run the daemon against the agent (no servos attached, mock UART):"
	@echo "     ./core/build/src/waypoint-core --socket /tmp/waypoint-nats.sock --servo-mock"
	@echo "   Expect a heartbeat log line within 1 s and event.mode=manual published."
	@echo
	@echo "5. From dashboard: drive the on-screen joystick; watch core log cmd.drive arrivals"
	@echo "   and telemetry.drive publications at 50 Hz."
	@echo
	@echo "Pi hardware smoke (deferred): see docs/setup/core.md."

.PHONY: firmware-build firmware-test firmware-flash firmware-smoke

firmware-build:
	cd firmware/servo-relay && idf.py build

firmware-test:
	$(MAKE) -C firmware/servo-relay/test check

firmware-flash:
	@if lsof /dev/ttyAMA0 >/dev/null 2>&1; then \
	  echo "Refusing to flash: /dev/ttyAMA0 is held."; \
	  echo "Stop waypoint-core first, then retry."; \
	  exit 1; \
	fi
	cd firmware/servo-relay && idf.py flash

firmware-smoke:
	@echo "ESP32 servo-relay bring-up checklist. Prerequisite: ESP-IDF v5.x sourced."
	@echo
	@echo "1. make firmware-test         # host-runnable parser tests"
	@echo "2. make firmware-build        # produces build/servo-relay.bin"
	@echo "3. Stop waypoint-core on the Pi."
	@echo "4. Plug USB-C into the HAT's Type_C2 port."
	@echo "5. make firmware-flash"
	@echo "6. Unplug USB-C, restart waypoint-core."
	@echo
	@echo "Then run the hardware bring-up steps A–H from docs/setup/firmware.md."

.PHONY: image-qemu image-qemu-boot image-qemu-ota image-qemu-rollback image-smoke

image-qemu:
	./image/scripts/qemu-run.sh

image-qemu-boot:
	@command -v expect >/dev/null || { echo "brew install expect"; exit 1; }
	./image/scripts/qemu-boot-smoke.exp

image-qemu-ota:
	@command -v expect >/dev/null || { echo "brew install expect"; exit 1; }
	@./image/scripts/serve-swu.sh & \
	  SERVER_PID=$$!; \
	  trap "kill $$SERVER_PID 2>/dev/null" EXIT; \
	  sleep 1; \
	  ./image/scripts/qemu-ota-smoke.exp

image-qemu-rollback:
	@command -v expect >/dev/null || { echo "brew install expect"; exit 1; }
	bash ./image/scripts/build-broken-swu.sh
	@./image/scripts/serve-swu.sh & \
	  SERVER_PID=$$!; \
	  trap "kill $$SERVER_PID 2>/dev/null" EXIT; \
	  sleep 1; \
	  ./image/scripts/qemu-rollback-smoke.exp

image-smoke:
	@echo "Phase 5 Mac smoke (full pipeline). Signing is cosign-keyless via CI;"
	@echo "QEMU smokes run with WAYPOINT_SKIP_VERIFY=1 (dev variant only)."
	@echo
	@echo "1. make -C image image-prod      # ~30 min cold, ~3 min ccache-warm"
	@echo "2. make image-qemu-boot          # boots image in QEMU, asserts agent+core active"
	@echo "3. make image-qemu-ota           # applies .swu, verifies A→B swap"
	@echo "4. make image-qemu-rollback      # broken .swu → auto-revert"
	@echo "5. ls -lh image/output/prod/images/*.swu"
	@echo
	@echo "Pi 5 hardware smoke checklist lives in docs/setup/image.md."

# Release lines. Versions and changelog entries derive from conventional
# commits via git-cliff (brew install git-cliff): feat bumps minor, fix
# bumps patch, a breaking change bumps major. Each line only sees commits
# touching the paths that ship in it; the targets prepend the new section
# to the line's changelog and print the commit/tag/push commands to run.
CLIFF_IMAGE_FLAGS := --tag-pattern '^image-v' \
  --include-path 'agent/**' --include-path 'core/**' \
  --include-path 'dashboard/**' --include-path 'protocol/**' \
  --include-path 'image/**'
CLIFF_PROXY_FLAGS := --tag-pattern '^proxy-v' \
  --include-path 'proxy/**' --include-path 'dashboard/**' \
  --include-path 'protocol/**'

release-image:
	@command -v git-cliff >/dev/null || { echo "git-cliff not found: brew install git-cliff"; exit 1; }
	@last=$$(git tag -l 'image-v*' --sort=-v:refname | head -1); \
	[ -n "$$last" ] || { echo "No image-v tag yet: the first release is tagged by hand (see docs/setup/image.md)."; exit 1; }; \
	next=$$(git-cliff $(CLIFF_IMAGE_FLAGS) --bumped-version 2>/dev/null); \
	case "$$next" in image-v*) ;; *) next="image-v$$next" ;; esac; \
	if [ "$$next" = "$$last" ]; then echo "No releasable changes since $$last."; exit 1; fi; \
	git-cliff $(CLIFF_IMAGE_FLAGS) --unreleased --tag "$$next" --prepend CHANGELOG.md 2>/dev/null; \
	echo "CHANGELOG.md updated for $$next ($$last -> $$next)."; \
	echo "Review the diff, then:"; \
	echo "  git add CHANGELOG.md && git commit -m \"chore(release): $$next\""; \
	echo "  git tag -m \"$$next\" $$next"; \
	echo "  git push origin main $$next"

release-proxy:
	@command -v git-cliff >/dev/null || { echo "git-cliff not found: brew install git-cliff"; exit 1; }
	@last=$$(git tag -l 'proxy-v*' --sort=-v:refname | head -1); \
	[ -n "$$last" ] || { echo "No proxy-v tag yet: the first release is tagged by hand (see docs/setup/image.md)."; exit 1; }; \
	next=$$(git-cliff $(CLIFF_PROXY_FLAGS) --bumped-version 2>/dev/null); \
	case "$$next" in proxy-v*) ;; *) next="proxy-v$$next" ;; esac; \
	if [ "$$next" = "$$last" ]; then echo "No releasable changes since $$last."; exit 1; fi; \
	git-cliff $(CLIFF_PROXY_FLAGS) --unreleased --tag "$$next" --prepend proxy/CHANGELOG.md 2>/dev/null; \
	echo "proxy/CHANGELOG.md updated for $$next ($$last -> $$next)."; \
	echo "Review the diff, then:"; \
	echo "  git add proxy/CHANGELOG.md && git commit -m \"chore(release): $$next\""; \
	echo "  git tag -m \"$$next\" $$next"; \
	echo "  git push origin main $$next"

auth-smoke:
	@echo "Phase 6 Mac smoke — five end-to-end checks."
	@echo
	@echo "Prerequisites: postgres up, proxy running, agent running, dashboard built or pnpm dev."
	@echo "  docker compose up -d postgres"
	@echo "  cd proxy && go run ./cmd/waypoint-proxy &"
	@echo "  cd agent && go run ./cmd/waypoint-agent &"
	@echo "  cd dashboard && pnpm dev"
	@echo
	@echo "1. Sign in to the proxy as admin. From the sim's local UI, drive briefly."
	@echo "2. Open /admin/audit — confirm command rows appear within ~2s."
	@echo "3. Stop the agent. Watch /admin/alerts (or the rover view) — proxy_disconnected"
	@echo "   should fire CRITICAL within ~2s."
	@echo "4. Restart the agent — the alert auto-resolves; the row clears from the active"
	@echo "   panel and lands in History."
	@echo "5. Invite a new user via /admin/users — copy the accept URL, sign in as them in"
	@echo "   a private window. Their fleet should be empty (no grants yet)."
	@echo "6. As admin, grant the new user 'monitor' on sim-01 via the AccessMatrix."
	@echo "   The new user reloads — sim-01 visible; joystick HIDDEN; attempting cmd.*"
	@echo "   via browser devtools NATS publish returns -ERR (NATS-level rejection)."
	@echo
	@echo "All five green → Phase 6 + v1 complete on Mac."
