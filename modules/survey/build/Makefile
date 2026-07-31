# Run from modules/survey: `make -f build/Makefile raw`.
# The Go binary is cross-compiled on the host; the rootfs assembly and
# mksquashfs run inside Docker so the build works on macOS.
MODULE_ID := survey
VERSION   ?= 0.1.0
OUT       := dist/$(MODULE_ID)-$(VERSION).raw

.PHONY: all raw bin clean

all: raw
raw: $(OUT)

bin:
	mkdir -p build/bin
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" \
	  -o build/bin/waypoint-module-$(MODULE_ID) ./cmd/waypoint-module-$(MODULE_ID)

$(OUT): bin
	mkdir -p dist
	docker buildx build --platform linux/arm64 --build-arg VERSION=$(VERSION) \
	  -f build/Dockerfile --target export -o type=local,dest=dist .

clean:
	rm -rf build/bin dist
