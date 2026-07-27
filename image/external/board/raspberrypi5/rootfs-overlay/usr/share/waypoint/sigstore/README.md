# Sigstore trusted root

`trusted_root.json` ships in the built image at
`/usr/share/waypoint/sigstore/trusted_root.json`. It is a pinned Sigstore TUF
snapshot (the public-good Fulcio/Rekor/CTFE/TSA public keys) used for offline
module signature verification; the agent loads it via `modverify.NewFromFile`.

**The file is committed to the repo** so that the rover's root of trust is
reproducible, reviewable in git, and buildable offline. Buildroot copies it
verbatim into the rootfs via `BR2_ROOTFS_OVERLAY`.

Regenerate it (initial creation, or when Sigstore rotates its roots — rare)
with cosign v2.4+:

```sh
cosign initialize
cp ~/.sigstore/root/tuf-repo-cdn.sigstore.dev/targets/trusted_root.json \
   image/external/board/raspberrypi5/rootfs-overlay/usr/share/waypoint/sigstore/
```

Then commit the result. A valid root is several KB with `tlogs`,
`certificateAuthorities`, `ctlogs`, and `timestampAuthorities` sections — sanity
check it before committing.

> **Do not run `cosign trusted-root create` with no flags.** It builds an
> *empty* root (just the `mediaType` header, ~74 bytes) meant to be populated
> from a private Sigstore instance's cert files. Baking that into the image
> makes every signature verification fail at runtime.
