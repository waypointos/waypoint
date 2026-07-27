################################################################################
# waypoint-resize
#
# SITE_METHOD=local: source lives in this repo under src/. Buildroot
# caches extract+build stamps off package name+version, so source edits
# only propagate when the version advances (for image-prod) or when the
# build dir is wiped (for image-dev — see image/Makefile's
# WAYPOINT_LOCAL_DIRCLEAN). Bump VERSION for releases; let image-dev
# handle it for local iteration.
################################################################################

WAYPOINT_RESIZE_VERSION = 1.0.0
WAYPOINT_RESIZE_SITE = $(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-resize/src
WAYPOINT_RESIZE_SITE_METHOD = local
WAYPOINT_RESIZE_LICENSE = Apache-2.0

define WAYPOINT_RESIZE_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-resize/src/resize-store.sh \
		$(TARGET_DIR)/usr/bin/waypoint-resize-store
	$(INSTALL) -D -m 0644 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-resize/waypoint-resize.service \
		$(TARGET_DIR)/usr/lib/systemd/system/waypoint-resize.service
	mkdir -p $(TARGET_DIR)/etc/systemd/system/multi-user.target.wants
	ln -sf ../../../../usr/lib/systemd/system/waypoint-resize.service \
		$(TARGET_DIR)/etc/systemd/system/multi-user.target.wants/waypoint-resize.service
endef

$(eval $(generic-package))
