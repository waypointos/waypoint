################################################################################
# waypoint-firstboot
#
# SITE_METHOD=local: source lives in this repo under src/. Buildroot
# caches extract+build stamps off package name+version, so source edits
# only propagate when the version advances (for image-prod) or when the
# build dir is wiped (for image-dev — see image/Makefile's
# WAYPOINT_LOCAL_DIRCLEAN). Bump VERSION for releases; let image-dev
# handle it for local iteration.
################################################################################

WAYPOINT_FIRSTBOOT_VERSION = 1.2.0
WAYPOINT_FIRSTBOOT_SITE = $(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-firstboot/src
WAYPOINT_FIRSTBOOT_SITE_METHOD = local
WAYPOINT_FIRSTBOOT_LICENSE = Apache-2.0

define WAYPOINT_FIRSTBOOT_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-firstboot/src/firstboot-identity.sh \
		$(TARGET_DIR)/usr/bin/waypoint-firstboot-identity
	$(INSTALL) -D -m 0755 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-firstboot/src/boot-modules-sync.sh \
		$(TARGET_DIR)/usr/bin/waypoint-boot-modules-sync
	$(INSTALL) -D -m 0644 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-firstboot/waypoint-firstboot.service \
		$(TARGET_DIR)/usr/lib/systemd/system/waypoint-firstboot.service
	$(INSTALL) -D -m 0644 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-firstboot/waypoint-boot-modules-sync.service \
		$(TARGET_DIR)/usr/lib/systemd/system/waypoint-boot-modules-sync.service
	mkdir -p $(TARGET_DIR)/etc/systemd/system/multi-user.target.wants
	ln -sf ../../../../usr/lib/systemd/system/waypoint-firstboot.service \
		$(TARGET_DIR)/etc/systemd/system/multi-user.target.wants/waypoint-firstboot.service
	ln -sf ../../../../usr/lib/systemd/system/waypoint-boot-modules-sync.service \
		$(TARGET_DIR)/etc/systemd/system/multi-user.target.wants/waypoint-boot-modules-sync.service
endef

$(eval $(generic-package))
