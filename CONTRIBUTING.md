# Contributing to Waypoint

Thanks for your interest in improving Waypoint. Issues, fixes, and features are
all welcome.

## Contributor License Agreement

Contributions are accepted under the [Contributor License Agreement](CLA.md).
You keep the copyright to your work; the CLA grants the project the licenses it
needs and the ability to relicense in the future.

The first time you open a pull request, an automated check will ask you to sign.
You sign by posting this comment on the pull request:

```
I have read the CLA Document and I hereby sign the CLA
```

The check records your signature and turns green. You only sign once; later
pull requests are recognized automatically.

## Before you open a pull request

- Read [`CLAUDE.md`](CLAUDE.md) for the project's hard rules and conventions.
- UI work: build only from the design system in `dashboard/src/ui/` (see
  `dashboard/src/ui/README.md`). No Tailwind/MUI/Chakra, no inline hex colors.
- Protocol work: update `protocol/subjects.toml` and the relevant `.proto`, then
  regenerate bindings and update every consumer.
- C++ `core` touches the safety-critical control path; change it deliberately.

## Quality checks

Run the relevant suite before pushing:

```sh
make test          # full cross-language suite
go test ./...      # from agent/ or sim/ for Go changes
make core-test     # C++ core
```

Keep pull requests focused, with a descriptive branch name and a clear summary
of what changed and why.
