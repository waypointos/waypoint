#!/usr/bin/env bash
# module-attach-check.sh — empirically validate the systemd-portable attach path
# that the agent's module reconciler assumes. Run on the Pi 5 (or any systemd
# Linux host); the dev Mac cannot exercise portablectl.
#
# This script does NOT trust the agent's current code. It probes the mechanics
# and prints a verdict, because two agent assumptions are unverified on hardware:
#
#   1. Attach invocation. portable.go runs a bare
#        portablectl attach --copy=symlink <raw>
#      with no prefix. Module images declare PORTABLE_PREFIXES=waypoint-module and
#      ship a unit named waypoint-module-<id>.service, but the image filename is
#      <id>_<version>.raw, so portablectl's *default* prefix is <id> — which is
#      neither listed in PORTABLE_PREFIXES nor a match for the unit. The fix is
#      almost certainly an explicit "waypoint-module" prefix argument; this script
#      proves it.
#   2. Content access. reconciler.go reads the manifest from
#        /run/portables/<id>/usr/share/waypoint/modules/<id>/module.toml
#      and gateway/modulehttp.go serves static panels from
#        /run/portables/<id>/dashboard/ .
#      Both assume the image rootfs is globally mounted at /run/portables/<id>/.
#      portablectl may instead expose contents only inside the unit's mount
#      namespace. This script reports where the files are actually readable.
#
# Usage:
#   sudo ./module-attach-check.sh                      # synthetic image, attach mechanics
#   sudo ./module-attach-check.sh --image power-monitor_0.1.0.raw
#   sudo ./module-attach-check.sh --image X.raw --port 8090 --start   # also start + curl
#
# Attaches are transient (--runtime): they live in /run and vanish on reboot, and
# the script detaches on exit. It will not write to /etc.

# No `set -e`: probes deliberately run commands that fail (Probe B expects a
# rejection), and rc is captured + branched on explicitly via cap()/LAST_RC.
set -uo pipefail

# ---- args -------------------------------------------------------------------
IMAGE=""
PORT=""
DO_START=0
while [[ $# -gt 0 ]]; do
	case "$1" in
		--image) IMAGE="${2:-}"; shift 2 ;;
		--port)  PORT="${2:-}"; shift 2 ;;
		--start) DO_START=1; shift ;;
		-h|--help) sed -n '2,32p' "$0"; exit 0 ;;
		*) echo "unknown arg: $1" >&2; exit 2 ;;
	esac
done

# ---- output helpers ---------------------------------------------------------
if [[ -t 1 ]]; then B=$'\e[1m'; G=$'\e[32m'; R=$'\e[31m'; Y=$'\e[33m'; D=$'\e[2m'; N=$'\e[0m'
else B=""; G=""; R=""; Y=""; D=""; N=""; fi
section() { printf '\n%s== %s ==%s\n' "$B" "$1" "$N"; }
ok()    { printf '  %s[ OK ]%s %s\n' "$G" "$N" "$1"; }
fail()  { printf '  %s[FAIL]%s %s\n' "$R" "$N" "$1"; }
warn()  { printf '  %s[ ?? ]%s %s\n' "$Y" "$N" "$1"; }
info()  { printf '  %s[info]%s %s\n' "$D" "$N" "$1"; }
dump()  { sed 's/^/        /' ; }   # indent captured command output

# Run a command, capture rc + combined output without aborting under set -e.
LAST_OUT=""; LAST_RC=0
# Capture combined output + rc without aborting (works even if errexit is on).
cap() { LAST_OUT="$("$@" 2>&1)" && LAST_RC=0 || LAST_RC=$?; return 0; }

need() { command -v "$1" >/dev/null 2>&1; }

# ---- preconditions ----------------------------------------------------------
section "Preconditions"
[[ $EUID -eq 0 ]] || { fail "must run as root (portablectl + mount need it). Re-run with sudo."; exit 1; }
ok "running as root"
need portablectl && ok "portablectl present" || { fail "portablectl not found — install systemd-container"; exit 1; }
HAVE_DISSECT=0; need systemd-dissect && { HAVE_DISSECT=1; ok "systemd-dissect present (content-extraction probe enabled)"; } || warn "systemd-dissect absent — content-extraction probe skipped"
HAVE_MKSQUASH=0; need mksquashfs && { HAVE_MKSQUASH=1; ok "mksquashfs present (synthetic .raw build enabled)"; } || warn "mksquashfs absent — synthetic image will be a directory, not a .raw"

# ---- build a synthetic Waypoint-shaped portable -----------------------------
# Mirrors a real module: filename <id>_<ver>.raw, ID=waypoint-module-<id>,
# PORTABLE_PREFIXES=waypoint-module, unit waypoint-module-<id>.service, plus the
# module.toml + dashboard/panel.js the agent expects to read post-attach.
WORK="$(mktemp -d /tmp/waypoint-attach-check.XXXXXX)"
SYNTH_ID="check"
SYNTH_VER="0.0.1"
trap 'cleanup' EXIT

build_synth_tree() {
	local root="$1"
	mkdir -p "$root/usr/lib/systemd/system" \
	         "$root/usr/lib" \
	         "$root/usr/share/waypoint/modules/$SYNTH_ID" \
	         "$root/dashboard"
	cat > "$root/usr/lib/os-release" <<EOF
NAME="Waypoint Module: ${SYNTH_ID}"
ID=waypoint-module-${SYNTH_ID}
VERSION_ID=${SYNTH_VER}
PORTABLE_PREFIXES=waypoint-module
EOF
	# Type=oneshot /bin/true so the unit *could* run with RootDirectory= against
	# the host (no real rootfs in the synthetic image). We only inspect/link it.
	cat > "$root/usr/lib/systemd/system/waypoint-module-${SYNTH_ID}.service" <<EOF
[Unit]
Description=Waypoint module — ${SYNTH_ID} (synthetic attach check)

[Service]
Type=oneshot
ExecStart=/bin/true

[Install]
WantedBy=multi-user.target
EOF
	printf 'name = "%s"\n' "$SYNTH_ID" > "$root/usr/share/waypoint/modules/$SYNTH_ID/module.toml"
	printf '// synthetic panel\nexport default { mount(){} };\n' > "$root/dashboard/panel.js"
}

if [[ -n "$IMAGE" ]]; then
	[[ -f "$IMAGE" ]] || { fail "--image $IMAGE not found"; exit 1; }
	# Derive the portable name the way systemd does: basename, strip .raw, cut at first '_'.
	NAME="$(basename "$IMAGE")"; NAME="${NAME%.raw}"; NAME="${NAME%%_*}"
	ID="$NAME"
	IMG_PATH="$(readlink -f "$IMAGE")"
	section "Target: real image"
	info "image       = $IMG_PATH"
	info "derived name= $NAME   (portablectl default prefix / detach name)"
	info "expected unit= waypoint-module-$ID.service"
else
	NAME="$SYNTH_ID"; ID="$SYNTH_ID"
	section "Target: synthetic image"
	TREE="$WORK/tree"
	build_synth_tree "$TREE"
	if [[ $HAVE_MKSQUASH -eq 1 ]]; then
		IMG_PATH="$WORK/${SYNTH_ID}_${SYNTH_VER}.raw"
		mksquashfs "$TREE" "$IMG_PATH" -quiet -noappend -comp zstd >/dev/null
		info "built synthetic .raw: $IMG_PATH"
	else
		IMG_PATH="$WORK/${SYNTH_ID}_${SYNTH_VER}"
		mv "$TREE" "$IMG_PATH"   # directory attach; name still <id>_<ver>
		info "built synthetic directory image: $IMG_PATH  (mount findings may differ from a .raw)"
	fi
	info "derived name= $NAME ; expected unit= waypoint-module-$ID.service"
fi

ATTACHED=""   # set to the name portablectl registered, so cleanup can detach it
cleanup() {
	if [[ -n "$ATTACHED" ]]; then
		systemctl stop "waypoint-module-${ID}.service" >/dev/null 2>&1 || true
		portablectl detach --runtime "$ATTACHED" >/dev/null 2>&1 || true
	fi
	[[ -n "${DISSECT_MNT:-}" && -d "${DISSECT_MNT:-}" ]] && { umount "$DISSECT_MNT" >/dev/null 2>&1 || true; }
	rm -rf "$WORK" >/dev/null 2>&1 || true
}

# ---- PROBE A: inspect -------------------------------------------------------
section "Probe A — inspect (what portablectl sees in the image)"
cap portablectl inspect "$IMG_PATH"
if [[ $LAST_RC -eq 0 ]]; then ok "portablectl inspect succeeded"; else fail "portablectl inspect rc=$LAST_RC"; fi
printf '%s\n' "$LAST_OUT" | dump

# ---- PROBE B: faithful reproduction of the agent's CURRENT invocation -------
section "Probe B — agent's current call: bare attach, no prefix (EXPECTED to fail)"
info "command: portablectl attach --runtime --copy=symlink $IMG_PATH"
cap portablectl attach --runtime --copy=symlink "$IMG_PATH"
BARE_RC=$LAST_RC
if [[ $BARE_RC -eq 0 ]]; then
	warn "bare attach SUCCEEDED — unexpected; the default prefix matched. Registering for cleanup."
	ATTACHED="$NAME"
	portablectl detach --runtime "$NAME" >/dev/null 2>&1 || true
	ATTACHED=""
else
	ok "bare attach failed as predicted (rc=$BARE_RC) — confirms portable.go must pass a prefix"
fi
printf '%s\n' "$LAST_OUT" | dump

# ---- PROBE C: hypothesized fix — explicit waypoint-module prefix ------------
section "Probe C — fix: attach with explicit 'waypoint-module' prefix"
info "command: portablectl attach --runtime --copy=symlink $IMG_PATH waypoint-module"
cap portablectl attach --runtime --copy=symlink "$IMG_PATH" waypoint-module
PREFIX_RC=$LAST_RC
printf '%s\n' "$LAST_OUT" | dump
if [[ $PREFIX_RC -ne 0 ]]; then
	fail "prefixed attach failed (rc=$PREFIX_RC) — the prefix hypothesis is wrong; read the error above"
else
	ATTACHED="$NAME"
	ok "prefixed attach succeeded"

	# C.1 — under what name is it registered? (detach + readAttachedManifest key on this)
	cap portablectl list
	printf '%s\n' "$LAST_OUT" | dump
	if printf '%s\n' "$LAST_OUT" | grep -qE "(^|[[:space:]])${NAME}([[:space:]]|$|\.raw)"; then
		ok "registered as '${NAME}' — matches the <id> that detach()/manifest-read assume"
	else
		warn "could not confirm '${NAME}' in 'portablectl list' — check the name above; detach() uses <id>='${ID}'"
	fi

	# C.2 — did the unit get linked into the systemd view?
	cap systemctl is-enabled "waypoint-module-${ID}.service"
	if systemctl cat "waypoint-module-${ID}.service" >/dev/null 2>&1; then
		ok "unit waypoint-module-${ID}.service is known to systemd (matched + linked)"
	else
		fail "unit waypoint-module-${ID}.service NOT visible — prefix matched the image but not this unit name"
	fi

	# C.3 — is the image rootfs globally mounted at /run/portables/<id>/ ?
	section "Probe C.3 — where is image content actually readable?"
	info "agent assumes: /run/portables/${ID}/usr/share/... and /run/portables/${ID}/dashboard/"
	if [[ -d /run/portables ]]; then
		info "contents of /run/portables:"; ls -la /run/portables 2>&1 | dump
	else
		warn "/run/portables does not exist"
	fi
	MANIFEST_AT=""
	for cand in \
		"/run/portables/${ID}/usr/share/waypoint/modules/${ID}/module.toml" \
		"/run/portables/${NAME}/usr/share/waypoint/modules/${ID}/module.toml" \
		"/run/portables/${NAME}.raw/usr/share/waypoint/modules/${ID}/module.toml"; do
		[[ -f "$cand" ]] && { MANIFEST_AT="$cand"; break; }
	done
	if [[ -z "$MANIFEST_AT" ]]; then
		FOUND="$(find /run/portables /var/lib/portables -name module.toml 2>/dev/null | head -1 || true)"
		[[ -n "$FOUND" ]] && MANIFEST_AT="$FOUND"
	fi
	if [[ -n "$MANIFEST_AT" ]]; then
		ok "module.toml readable at: $MANIFEST_AT"
		[[ "$MANIFEST_AT" == "/run/portables/${ID}/usr/share/waypoint/modules/${ID}/module.toml" ]] \
			&& ok "matches reconciler.go's hard-coded path — no agent change needed for manifest read" \
			|| warn "DIFFERS from reconciler.go's /run/portables/<id>/usr/share/... — agent read path must change"
	else
		fail "module.toml NOT found on any global path — image rootfs is not exposed globally (namespace-only)"
		warn "=> reconciler.go's /run/portables/<id>/ manifest read cannot work; agent must extract from the .raw"
	fi
	# dashboard/panel.js (static-serving path in modulehttp.go)
	PANEL_AT=""
	for cand in "/run/portables/${ID}/dashboard/panel.js" "/run/portables/${NAME}/dashboard/panel.js"; do
		[[ -f "$cand" ]] && { PANEL_AT="$cand"; break; }
	done
	[[ -z "$PANEL_AT" ]] && PANEL_AT="$(find /run/portables -path '*/dashboard/panel.js' 2>/dev/null | head -1 || true)"
	if [[ -n "$PANEL_AT" ]]; then
		ok "dashboard/panel.js readable at: $PANEL_AT"
	else
		warn "dashboard/panel.js NOT found globally — modulehttp.go static serving from /run/portables/<id>/dashboard/ likely broken"
	fi

	# C.4 — optionally start the unit + curl the proxy port (real images only)
	if [[ $DO_START -eq 1 ]]; then
		section "Probe C.4 — start unit + smoke the port"
		cap systemctl start "waypoint-module-${ID}.service"
		if [[ $LAST_RC -eq 0 ]]; then ok "started waypoint-module-${ID}.service"; else fail "start rc=$LAST_RC"; printf '%s\n' "$LAST_OUT" | dump; fi
		sleep 1
		cap systemctl is-active "waypoint-module-${ID}.service"
		[[ "$LAST_OUT" == "active" ]] && ok "unit is active" || warn "is-active: $LAST_OUT"
		if [[ -n "$PORT" ]] && need curl; then
			cap curl -fsS -m 3 -o /dev/null -w '%{http_code}' "http://127.0.0.1:${PORT}/"
			[[ "$LAST_OUT" =~ ^[23] ]] && ok "curl 127.0.0.1:${PORT}/ → HTTP $LAST_OUT" || warn "curl 127.0.0.1:${PORT}/ → '$LAST_OUT' (rc=$LAST_RC)"
		fi
	fi
fi

# ---- PROBE D: robust content extraction straight from the .raw -------------
# The likely agent fix for content access if /run/portables/<id>/ is not global.
if [[ $HAVE_DISSECT -eq 1 && "$IMG_PATH" == *.raw ]]; then
	section "Probe D — read content directly from the .raw (the namespace-independent path)"
	cap systemd-dissect --copy-from "$IMG_PATH" "/usr/share/waypoint/modules/${ID}/module.toml" /dev/stdout
	if [[ $LAST_RC -eq 0 ]]; then
		ok "systemd-dissect --copy-from extracted module.toml — agent can read the manifest this way regardless of mounts"
		printf '%s\n' "$LAST_OUT" | dump
	else
		warn "systemd-dissect --copy-from rc=$LAST_RC"; printf '%s\n' "$LAST_OUT" | dump
	fi
	DISSECT_MNT="$WORK/mnt"; mkdir -p "$DISSECT_MNT"
	cap systemd-dissect --mount --read-only "$IMG_PATH" "$DISSECT_MNT"
	if [[ $LAST_RC -eq 0 ]]; then
		ok "systemd-dissect --mount works → image readable at a stable path (agent could mount + serve from here)"
		ls "$DISSECT_MNT/dashboard" 2>&1 | dump || true
		umount "$DISSECT_MNT" >/dev/null 2>&1 || true; DISSECT_MNT=""
	else
		warn "systemd-dissect --mount rc=$LAST_RC"; printf '%s\n' "$LAST_OUT" | dump
	fi
fi

# ---- VERDICT ----------------------------------------------------------------
section "VERDICT — what the agent must change"
if [[ ${BARE_RC:-1} -ne 0 && ${PREFIX_RC:-1} -eq 0 ]]; then
	ok "Attach: portable.go MUST pass the explicit prefix. Change Attach() to:"
	printf '        portablectl attach --copy=symlink <raw> waypoint-module\n'
	info "(consider --runtime too: modules are re-derived from desired state on boot, so a transient attach avoids stale /etc symlinks. Optional — confirm with the team.)"
elif [[ ${BARE_RC:-1} -eq 0 ]]; then
	warn "Attach: the bare call worked here — default prefix matched. Re-check on the real .raw before concluding."
else
	fail "Attach: neither bare nor prefixed attach succeeded — read Probes B/C output; the unit/prefix shape needs rethinking."
fi
printf '\n'
info "Detach name: agent calls 'portablectl detach <id>'. Confirm <id>='${ID}' equals the 'portablectl list' name above."
info "Content access: see Probe C.3 — if module.toml/panel.js were NOT under /run/portables/<id>/, then"
info "  reconciler.go (readAttachedManifest) and gateway/modulehttp.go (mountRoot) must switch to either"
info "  systemd-dissect --copy-from (manifest) and a stable --mount path or copied-out dir (static assets)."
printf '\nDone. Paste this whole output back to continue the Phase C agent corrections.\n'
