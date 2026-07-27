################################################################################
# waypoint-agent
################################################################################

WAYPOINT_AGENT_VERSION = $(or $(WAYPOINT_VERSION),0.5.0-dev)
WAYPOINT_AGENT_SITE = /src/agent
WAYPOINT_AGENT_SITE_METHOD = local
WAYPOINT_AGENT_LICENSE = Apache-2.0
WAYPOINT_AGENT_GOMOD = github.com/waypointos/waypoint/agent
# Buildroot's golang-package macro derives the installed binary name from
# the last path component of BUILD_TARGETS — `cmd/waypoint-agent` installs
# as `/usr/bin/waypoint-agent`. INSTALL_BINS was removed in 2026.02.
WAYPOINT_AGENT_BUILD_TARGETS = cmd/waypoint-agent

define WAYPOINT_AGENT_INSTALL_INIT_SYSTEMD
	$(INSTALL) -D -m 0644 \
		$(BR2_EXTERNAL_WAYPOINT_PATH)/package/waypoint-agent/waypoint-agent.service \
		$(TARGET_DIR)/usr/lib/systemd/system/waypoint-agent.service
	mkdir -p $(TARGET_DIR)/etc/systemd/system/multi-user.target.wants
	ln -sf ../../../../usr/lib/systemd/system/waypoint-agent.service \
		$(TARGET_DIR)/etc/systemd/system/multi-user.target.wants/waypoint-agent.service
endef

$(eval $(golang-package))
