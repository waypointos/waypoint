# swupdate

The `.swu` cpio descriptor template lives at `sw-description.in`. Tokens
(`{WAYPOINT_VERSION}`, `{WAYPOINT_VARIANT}`, `{ROOTFS_SHA256}`) are
substituted by `image/external/board/raspberrypi5/post-image.sh` before
the cpio is sealed.

## Trust model

Image artifacts are signed by **cosign keyless** at CI time. There is no
long-lived signing key in this repo or in any GitHub secret. The CI
workflow (`.github/workflows/image-build.yml`) uses GitHub's OIDC token
to obtain a short-lived Fulcio certificate; the resulting Sigstore
bundle (`<artifact>.cosign`) is published alongside the `.swu` in the
GitHub Release.

The rover's agent verifies the bundle with the `sigstore-go` library
before invoking `/usr/bin/swupdate`. The verifier asserts:

- the Fulcio certificate's `Subject` matches the regex
  `^https://github\.com/waypointos/waypoint/\.github/workflows/image-build\.yml@refs/tags/image-v.+$`,
- the OIDC issuer is `https://token.actions.githubusercontent.com`,
- Rekor has an inclusion proof for the signature.

swupdate's own signature verification is intentionally disabled —
cosign is the single trust gate. Defense-in-depth via swupdate-side
verification can be re-added later (Phase 6+) without disrupting the
cosign path.

## Rotation

Rotation is automatic: every workflow run uses a fresh ephemeral key.
There is nothing to rotate, no bridge image to ship.

## Dev / QEMU smoke

For QEMU smokes the `.swu` is built without a `.cosign` bundle. The
agent honours `WAYPOINT_SKIP_VERIFY=1` (only in dev-variant images) so
local smokes don't need a real Sigstore-signed artifact. Production
images do not honour that environment variable.
