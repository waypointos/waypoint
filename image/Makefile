SHELL := /bin/bash

DOCKER ?= docker
DOCKER_IMAGE := waypoint-buildroot:2026.02
SRC := $(abspath $(CURDIR)/..)

# Platform-aware output storage.
#
# On macOS hosts (Docker via OrbStack / Docker Desktop), Buildroot's
# heavy parallel writes hit virtiofs flush-visibility races — glibc's
# build loses subdirectory stamp files to the parent make's prereq
# scan non-deterministically. We avoid this by routing $O into a
# named Docker volume (backed by the container engine's native fs).
# After the build, only the final artifacts are copied out to the
# host bind mount so `make image-qemu-*` keeps working.
#
# On Linux hosts (GitHub Actions, native dev), bind mounts are POSIX-
# correct and fast. We use the host path directly so actions/cache can
# snapshot the toolchain/kernel/system-lib state between CI runs.
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
  USE_VOLUME := 1
  DOCKER_VOLUME := waypoint-image-build
  OUTPUT_MOUNT := -v $(DOCKER_VOLUME):/work/output-internal
  BR_O_PROD := /work/output-internal/prod
  BR_O_DEV  := /work/output-internal/dev
  COPY_OUT_PROD := mkdir -p /work/output/prod/images && cp -a /work/output-internal/prod/images/. /work/output/prod/images/
  COPY_OUT_DEV  := mkdir -p /work/output/dev/images  && cp -a /work/output-internal/dev/images/.  /work/output/dev/images/
else
  USE_VOLUME :=
  DOCKER_VOLUME :=
  OUTPUT_MOUNT :=
  BR_O_PROD := /work/output/prod
  BR_O_DEV  := /work/output/dev
  COPY_OUT_PROD := true
  COPY_OUT_DEV  := true
endif
# Job count resolved on the host (Mac or Linux) so the container never
# needs nproc. Override with `make image-prod JOBS=N`.
JOBS ?= $(shell sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4)
# Run the container as the host user so:
#  (a) Buildroot is not running as root — several autotools packages
#      (host-tar, coreutils, …) refuse to configure as root because
#      privilege-dependent feature checks would lie at runtime.
#  (b) Files Buildroot writes under image/output, image/ccache, image/dl
#      are owned by the host user, so `make clean` etc. don't need sudo.
USER_FLAGS ?= --user $(shell id -u):$(shell id -g)

# Extra flags for the build container. Empty by default; set it when the
# build shares a host with other work, e.g.
# `make image-prod JOBS=6 DOCKER_RUN_EXTRA=--cpus=7`.
DOCKER_RUN_EXTRA ?=

# Local Buildroot packages whose sources live in this repo (SITE_METHOD=local).
# Each one keeps a stable VERSION across iterations, which means Buildroot's
# extract/build stamps cache aggressively — source edits do NOT propagate
# unless the version moves or the build dir is wiped. image-dev wipes them
# (via per-package -dirclean) before every build so the dev iteration loop
# is "edit source, make image-dev, reflash" with no version-bump bookkeeping.
# image-prod intentionally does NOT wipe stamps: prod relies on stable
# versions + ccache for reproducibility, and source changes that need to
# ship are gated by a deliberate WAYPOINT_VERSION bump.
WAYPOINT_LOCAL_PKGS := waypoint-agent waypoint-core waypoint-bootcheck waypoint-firstboot waypoint-resize
WAYPOINT_LOCAL_DIRCLEAN := $(addsuffix -dirclean,$(WAYPOINT_LOCAL_PKGS))
# Dircleaned every image-dev so kconfig sub-options, kernel fragments,
# and install-hook toggles take effect — Buildroot's stamp cache doesn't
# invalidate on those.
WAYPOINT_UPSTREAM_REBUILD_PKGS := \
  linux \
  rpi-firmware \
  gstreamer1 \
  gst1-plugins-base \
  gst1-plugins-good \
  gst1-plugins-bad \
  gst1-plugins-ugly \
  libv4l
WAYPOINT_UPSTREAM_DIRCLEAN := $(addsuffix -dirclean,$(WAYPOINT_UPSTREAM_REBUILD_PKGS))

.PHONY: docker-image image image-prod image-dev image-dev-clean menuconfig clean build-volume vendor-agent embed-dashboard

docker-image:
	$(DOCKER) build -f Dockerfile.buildroot -t $(DOCKER_IMAGE) .

# Buildroot's package/go/go.mk hard-sets GOFLAGS=-mod=vendor for every
# Go package, so the build has no network dependency. Our agent uses
# go modules with a local replace directive; we vendor on the host
# (where Go + internet are available) before Buildroot rsyncs the
# source into the container. Re-runs on every image-prod/image-dev so
# go.mod changes always propagate.
vendor-agent:
	cd $(SRC)/agent && go mod vendor

# Refresh agent/web/dist so the agent's //go:embed picks up the current
# dashboard source. Without this prereq, image builds embed whatever
# happens to be sitting in agent/web/dist from the last local run, which
# goes stale the moment dashboard sources change. Runs on the host (pnpm
# is required there anyway for dev); --frozen-lockfile keeps fresh CI
# checkouts working without a separate install step.
embed-dashboard:
	cd $(SRC)/dashboard && pnpm install --frozen-lockfile && pnpm build
	rm -rf $(SRC)/agent/web/dist
	mkdir -p $(SRC)/agent/web/dist
	cp -r $(SRC)/dashboard/dist/. $(SRC)/agent/web/dist/

# Idempotent: creates the named volume on first run if we're using one
# (Darwin path), no-op on Linux. The Dockerfile pre-populates
# /work/output-internal with chmod 0777 so the first mount inherits
# world-writable perms.
build-volume:
ifeq ($(USE_VOLUME),1)
	@$(DOCKER) volume inspect $(DOCKER_VOLUME) >/dev/null 2>&1 \
	  || $(DOCKER) volume create $(DOCKER_VOLUME) >/dev/null
endif

image-prod: vendor-agent embed-dashboard docker-image build-volume
	mkdir -p output/prod/images ccache dl
	$(DOCKER) run --rm $(USER_FLAGS) $(DOCKER_RUN_EXTRA) \
	  -v $(SRC):/src \
	  $(OUTPUT_MOUNT) \
	  -v $(CURDIR)/output:/work/output \
	  -v $(CURDIR)/ccache:/work/ccache \
	  -v $(CURDIR)/dl:/work/dl \
	  -w /buildroot \
	  -e HOME=/work \
	  -e BR2_DL_DIR=/work/dl \
	  -e BR2_CCACHE_DIR=/work/ccache \
	  -e WAYPOINT_VERSION="$${WAYPOINT_VERSION:-0.0.0-dev}" \
	  -e WAYPOINT_VARIANT=prod \
	  -e WAYPOINT_EXTRA_OVERLAY="$${WAYPOINT_EXTRA_OVERLAY:-}" \
	  $(DOCKER_IMAGE) bash -c "\
	    set -euo pipefail && \
	    make BR2_EXTERNAL=/src/image/external O=$(BR_O_PROD) waypoint_prod_defconfig && \
	    make BR2_EXTERNAL=/src/image/external O=$(BR_O_PROD) -j$(JOBS) && \
	    $(COPY_OUT_PROD)"

image-dev: vendor-agent embed-dashboard docker-image build-volume
	mkdir -p output/dev/images ccache dl
	$(DOCKER) run --rm $(USER_FLAGS) $(DOCKER_RUN_EXTRA) \
	  -v $(SRC):/src \
	  $(OUTPUT_MOUNT) \
	  -v $(CURDIR)/output:/work/output \
	  -v $(CURDIR)/ccache:/work/ccache \
	  -v $(CURDIR)/dl:/work/dl \
	  -w /buildroot \
	  -e HOME=/work \
	  -e BR2_DL_DIR=/work/dl \
	  -e BR2_CCACHE_DIR=/work/ccache \
	  -e WAYPOINT_VERSION="$${WAYPOINT_VERSION:-0.0.0-dev}" \
	  -e WAYPOINT_VARIANT=dev \
	  -e WAYPOINT_EXTRA_OVERLAY="$${WAYPOINT_EXTRA_OVERLAY:-}" \
	  $(DOCKER_IMAGE) bash -c "\
	    set -euo pipefail && \
	    make BR2_EXTERNAL=/src/image/external O=$(BR_O_DEV) waypoint_dev_defconfig && \
	    make BR2_EXTERNAL=/src/image/external O=$(BR_O_DEV) $(WAYPOINT_LOCAL_DIRCLEAN) $(WAYPOINT_UPSTREAM_DIRCLEAN) && \
	    make BR2_EXTERNAL=/src/image/external O=$(BR_O_DEV) -j$(JOBS) && \
	    $(COPY_OUT_DEV)"

# Wipes the docker volume + output/dev for a full from-scratch image-dev.
# Use when WAYPOINT_UPSTREAM_REBUILD_PKGS doesn't catch a stale stamp.
image-dev-clean:
ifeq ($(USE_VOLUME),1)
	-$(DOCKER) volume rm $(DOCKER_VOLUME) 2>/dev/null || true
endif
	rm -rf output/dev
	$(MAKE) image-dev

menuconfig: docker-image build-volume
	$(DOCKER) run --rm -it $(USER_FLAGS) \
	  -v $(SRC):/src \
	  $(OUTPUT_MOUNT) \
	  -v $(CURDIR)/output:/work/output \
	  -w /buildroot \
	  -e HOME=/work \
	  $(DOCKER_IMAGE) make BR2_EXTERNAL=/src/image/external O=$(BR_O_PROD) menuconfig

image: image-prod

clean:
	rm -rf output
ifeq ($(USE_VOLUME),1)
	-$(DOCKER) volume rm $(DOCKER_VOLUME) 2>/dev/null || true
endif
