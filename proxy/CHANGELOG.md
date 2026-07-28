# Changelog

Generated from conventional commits by the release targets in the root
Makefile. Do not edit by hand.

## 0.17.0 (2026-07-28)

### Features

- **dashboard:** render module settings as fields from the manifest schema
- **proxy:** echo a version's config schema from its release manifest
- **protocol:** declare module config schema in the manifest

### Bug fixes

- **proxy:** relay component-class module commands

## 0.16.0 (2026-07-27)

### Features

- **dashboard:** add per-rover module config form

### Bug fixes

- **proxy:** keep per-rover module config on version-only updates

## 0.15.1 (2026-07-27)

### Bug fixes

- **modules:** stabilize module tab order across snapshots

## 0.15.0 (2026-07-27)

First release from the public repository; the proxy is now licensed
AGPL-3.0 while the rest of the platform stays Apache-2.0.

### Features

- **protocol:** add signed servo goal velocity op and retire cmd.drill

## 0.14.5 and earlier

Released before this changelog existed; see the `proxy-v*` tags. Derived
entries start with the next release cut by `make release-proxy`.
