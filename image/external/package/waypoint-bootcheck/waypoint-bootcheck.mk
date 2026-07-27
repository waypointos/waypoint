################################################################################
# waypoint-bootcheck
################################################################################

WAYPOINT_BOOTCHECK_VERSION = 1.0.0
WAYPOINT_BOOTCHECK_SITE = $(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-bootcheck/src
WAYPOINT_BOOTCHECK_SITE_METHOD = local
WAYPOINT_BOOTCHECK_LICENSE = Apache-2.0

define WAYPOINT_BOOTCHECK_BUILD_CMDS
	$(TARGET_CC) -O2 -Wall -o $(@D)/bootcheck $(@D)/bootcheck.c
endef

define WAYPOINT_BOOTCHECK_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/bootcheck $(TARGET_DIR)/usr/bin/waypoint-bootcheck
	$(INSTALL) -D -m 0755 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-bootcheck/src/bootcount-tick.sh \
		$(TARGET_DIR)/usr/bin/waypoint-bootcount-tick
	$(INSTALL) -D -m 0644 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-bootcheck/bootcheck.service \
		$(TARGET_DIR)/usr/lib/systemd/system/bootcheck.service
	$(INSTALL) -D -m 0644 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-bootcheck/bootcount-tick.service \
		$(TARGET_DIR)/usr/lib/systemd/system/bootcount-tick.service
	mkdir -p $(TARGET_DIR)/etc/systemd/system/multi-user.target.wants
	ln -sf ../../../../usr/lib/systemd/system/bootcheck.service \
		$(TARGET_DIR)/etc/systemd/system/multi-user.target.wants/bootcheck.service
	mkdir -p $(TARGET_DIR)/etc/systemd/system/sysinit.target.wants
	ln -sf ../../../../usr/lib/systemd/system/bootcount-tick.service \
		$(TARGET_DIR)/etc/systemd/system/sysinit.target.wants/bootcount-tick.service
endef

$(eval $(generic-package))
