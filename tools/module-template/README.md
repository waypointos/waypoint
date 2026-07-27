# Waypoint module template

A working starter for a no-rebuild Waypoint module. Copy this directory into a
new repo, rename `example` to your module id, and you have a daemon that
connects to the rover bus, answers health probes, publishes telemetry, and ships
a dashboard tab. Full guide: `docs/building-modules.md` in the waypoint repo.
The shipping reference is `waypoint-ubiquiti-mobile-router`.

## Layout

```
go.mod                            # the daemon module (built from cmd/)
module.toml                       # manifest
cmd/waypoint-module-example/      # daemon entrypoint
internal/                         # config + publisher packages
protocol/                         # module-owned proto + generated bindings
systemd/                          # the portable-service unit
build/                            # SEPARATE go module: .raw build + manifest.json
dashboard/                        # Vite/React panel, builds to dist/panel.js
.github/workflows/release.yml     # cosign-keyless release on v* tags
```

The repo has two Go modules: the root (daemon) and `build/` (the release
manifest generator). Run all build commands from the repo root.

## Rename checklist

Replace `example` everywhere when you start:

- `module.toml` (`name`, `label`, `entrypoint`, subjects, `tab_id`)
- `cmd/waypoint-module-example/` directory name
- `systemd/waypoint-module-example.service`
- `MODULE_ID` in `build/Makefile`
- the `go_package` + `package` in `protocol/example.proto`, then re-run
  `cd protocol && buf generate` and copy `gen/ts/<id>_pb.ts` to
  `dashboard/src/proto/`
- the asset names in `.github/workflows/release.yml`

## Build and test

```sh
go test ./...                                   # daemon
cd protocol && buf generate                     # regenerate bindings after a .proto change
cd dashboard && pnpm install && pnpm build      # -> dashboard/dist/panel.js
cd dashboard && pnpm test                       # panel
make -f build/Makefile raw VERSION=0.1.0        # -> dist/example-0.1.0.raw (needs mksquashfs)
```

`make raw` builds the arm64 daemon, stages the binary, manifest, unit, and
`dashboard/dist/panel.js`, and produces the squashfs `.raw`. Build the dashboard
panel first; the target hard-fails without it.

## Release and deploy

No signing key to manage. Signing is cosign keyless (Sigstore/Fulcio): CI mints
a short-lived GitHub OIDC token, and the proxy pins this repo's release-workflow
identity at registration time.

- `git tag v0.1.0 && git push --tags`
- CI builds the panel and `<id>-<version>.raw`, signs it, and publishes the
  `.raw`, `.raw.cosign` bundle, and `manifest.json` to a GitHub Release.
- In the proxy admin UI, register the repo URL, then enable the module per rover
  with its config. No image rebuild required.
