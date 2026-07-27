################################################################################
# waypoint-core
################################################################################

WAYPOINT_CORE_VERSION = $(or $(WAYPOINT_VERSION),0.5.0-dev)
WAYPOINT_CORE_SITE = /src/core
WAYPOINT_CORE_SITE_METHOD = local
WAYPOINT_CORE_LICENSE = Apache-2.0
WAYPOINT_CORE_DEPENDENCIES = protobuf

WAYPOINT_CORE_CONF_OPTS = \
	-DWP_CORE_BUILD_TESTS=OFF \
	-DWP_PROTO_ROOT=/src/protocol

define WAYPOINT_CORE_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/src/waypoint-core $(TARGET_DIR)/usr/bin/waypoint-core
	$(INSTALL) -D -m 0644 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-core/waypoint-core.service \
		$(TARGET_DIR)/usr/lib/systemd/system/waypoint-core.service
	mkdir -p $(TARGET_DIR)/etc/systemd/system/multi-user.target.wants
	ln -sf ../../../../usr/lib/systemd/system/waypoint-core.service \
		$(TARGET_DIR)/etc/systemd/system/multi-user.target.wants/waypoint-core.service
	install -d $(TARGET_DIR)/run/waypoint
	$(INSTALL) -D -m 0644 /src/protocol/platform/waypoint-rover.toml \
		$(TARGET_DIR)/usr/share/waypoint/platform.toml
endef

$(eval $(cmake-package))
