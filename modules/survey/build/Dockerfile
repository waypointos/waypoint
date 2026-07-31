# Builds the survey module portable image. The module root carries its own
# Python runtime because the portable service sees only this filesystem.
# Build context is modules/survey; the Go binary is cross-compiled by
# build/Makefile before docker runs.

FROM --platform=linux/arm64 debian:bookworm-slim AS rootfs
RUN apt-get update \
 && apt-get install -y --no-install-recommends python3 python3-pip libglib2.0-0 \
 && rm -rf /var/lib/apt/lists/*
RUN pip3 install --break-system-packages --no-cache-dir opencv-contrib-python-headless numpy
# Fail the build early if the aruco contrib module is missing.
RUN python3 -c "import cv2; cv2.aruco.getPredefinedDictionary(cv2.aruco.DICT_5X5_100); print('cv2', cv2.__version__)"
COPY build/bin/waypoint-module-survey /usr/bin/waypoint-module-survey
COPY module.toml /usr/share/waypoint/modules/survey/module.toml
COPY vision/detect.py /usr/share/waypoint/modules/survey/vision/detect.py
COPY systemd/waypoint-module-survey.service /usr/lib/systemd/system/waypoint-module-survey.service
# The agent serves /dashboard/* of the mounted image at /module/survey/static/.
COPY dashboard/dist/panel.js /dashboard/panel.js
COPY dashboard/dist/teleop.js /dashboard/teleop.js

FROM --platform=linux/arm64 debian:bookworm-slim AS assembler
ARG VERSION=0.0.0-dev
RUN apt-get update \
 && apt-get install -y --no-install-recommends squashfs-tools \
 && rm -rf /var/lib/apt/lists/*
COPY --from=rootfs / /staging
# Portable skeleton: systemd binds host /dev,/proc,/sys (+ /tmp,/var/tmp for
# PrivateTmp) onto these mount points; missing ones fail the unit with
# NAMESPACE/226. os-release must carry PORTABLE_PREFIXES for portablectl.
RUN mkdir -p /staging/dev /staging/proc /staging/sys /staging/run /staging/tmp \
             /staging/var/tmp /staging/etc /staging/root \
             /staging/usr/lib/extension-release.d \
 && : > /staging/etc/machine-id \
 && touch /staging/etc/resolv.conf \
 && printf 'NAME="Waypoint Module: survey"\nID=waypoint-module-survey\nVERSION_ID=%s\nPORTABLE_PREFIXES=waypoint-module\n' "$VERSION" \
      > /staging/usr/lib/os-release \
 && printf 'ID=waypoint-module-survey\nVERSION_ID=%s\nEXTENSION_RELOAD_MANAGER=1\n' "$VERSION" \
      > /staging/usr/lib/extension-release.d/extension-release.survey \
 && rm -rf /staging/var/lib/apt /staging/var/cache
RUN mkdir /out && mksquashfs /staging /out/survey-$VERSION.raw -comp zstd -no-progress -all-root

FROM scratch AS export
COPY --from=assembler /out/ /
