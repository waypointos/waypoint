# Run from the repo ROOT: `make -f build/Makefile raw`.
# All recipe paths are root-relative; `make -C build` will NOT work.
MODULE_ID := example
VERSION   ?= $(shell git describe --tags --always 2>/dev/null | sed 's/^v//' || echo 0.0.0-dev)
OUT       := dist/$(MODULE_ID)-$(VERSION).raw

.PHONY: all raw clean

all: raw
raw: $(OUT)

bin/waypoint-module-$(MODULE_ID):
	mkdir -p bin
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
	  -o bin/waypoint-module-$(MODULE_ID) ./cmd/waypoint-module-$(MODULE_ID)

# The dashboard panel.js must already be built (CI builds it before `raw`).
$(OUT): bin/waypoint-module-$(MODULE_ID)
	test -f dashboard/dist/panel.js || { echo "build the panel first: (cd dashboard && pnpm build)"; exit 1; }
	mkdir -p dist \
	  staging/usr/bin \
	  staging/usr/share/waypoint/modules/$(MODULE_ID) \
	  staging/usr/lib/extension-release.d \
	  staging/usr/lib/systemd/system \
	  staging/dashboard
# Minimal rootfs skeleton: a portable RootImage runs in a mount namespace, and
# systemd binds the host's /dev,/proc,/sys (+ /tmp,/var/tmp for PrivateTmp) onto
# these mount points. Without them the unit fails at step NAMESPACE (status 226).
	mkdir -p staging/dev staging/proc staging/sys staging/run staging/tmp staging/var/tmp staging/etc staging/root
	touch staging/etc/machine-id staging/etc/resolv.conf
	cp bin/waypoint-module-$(MODULE_ID) staging/usr/bin/
	cp module.toml staging/usr/share/waypoint/modules/$(MODULE_ID)/
	cp systemd/waypoint-module-$(MODULE_ID).service staging/usr/lib/systemd/system/
	cp dashboard/dist/panel.js staging/dashboard/panel.js
	bash build/gen-os-release.sh $(MODULE_ID) $(VERSION) > staging/usr/lib/os-release
	bash build/gen-extension-release.sh $(MODULE_ID) $(VERSION) > staging/usr/lib/extension-release.d/extension-release.$(MODULE_ID)
	mksquashfs staging $(OUT) -comp zstd -no-progress -all-root
	rm -rf staging

clean:
	rm -rf bin dist staging
