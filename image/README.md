# image/

Buildroot external tree for the Waypoint Pi 5 image. Run `make image-prod`
from this directory to build the production .img + .swu artifacts, or
`make image-dev` for the local iteration image (SSH, debug tools, mock
motors, skip-verify). Docker required on macOS.

Releases are cut with `make release-image` from the repo root: it derives
the next `image-v*` version and changelog entry from conventional commits,
and the pushed tag triggers the CI build + signing + GitHub Release.

See `docs/setup/image.md` for build prerequisites, the dev iteration loop,
signing key handling, and the Pi-hardware smoke recipe.
