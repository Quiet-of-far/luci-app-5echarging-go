include $(TOPDIR)/rules.mk

PKG_NAME:=luci-app-5echarging
PKG_VERSION:=1.0.1
PKG_RELEASE:=2

PKG_MAINTAINER:=Quiet
PKG_LICENSE:=MIT

include $(INCLUDE_DIR)/package.mk

define Package/$(PKG_NAME)
  SECTION:=luci
  CATEGORY:=LuCI
  SUBMENU:=3. Applications
  TITLE:=LuCI support for 5echarging
  PKGARCH:=all
  DEPENDS:=+luci-base +rpcd +uclient-fetch +ca-bundle
endef

define Package/$(PKG_NAME)/description
宿舍电费自动查询、预测与低余额提醒。Go 核心二进制通过 LuCI 在线下载或离线上传安装。
endef

define Build/Prepare
endef

define Build/Compile
endef

define Package/$(PKG_NAME)/install
	$(INSTALL_DIR) $(1)/usr/bin
	$(INSTALL_BIN) ./files/usr/bin/5echarging-uci2json $(1)/usr/bin/5echarging-uci2json

	$(INSTALL_DIR) $(1)/etc/config
	$(INSTALL_CONF) ./files/etc/config/5echarging $(1)/etc/config/5echarging

	$(INSTALL_DIR) $(1)/etc/init.d
	$(INSTALL_BIN) ./files/etc/init.d/5echarging $(1)/etc/init.d/5echarging

	$(INSTALL_DIR) $(1)/usr/lib/lua/luci/controller
	$(INSTALL_DATA) ./luasrc/controller/5echarging.lua $(1)/usr/lib/lua/luci/controller/5echarging.lua

	$(INSTALL_DIR) $(1)/usr/lib/lua/luci/model/cbi
	$(INSTALL_DATA) ./luasrc/model/cbi/5echarging.lua $(1)/usr/lib/lua/luci/model/cbi/5echarging.lua

	$(INSTALL_DIR) $(1)/usr/lib/lua/luci/view/5echarging
	$(INSTALL_DATA) ./luasrc/view/5echarging/status.htm $(1)/usr/lib/lua/luci/view/5echarging/status.htm

	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d
	$(INSTALL_DATA) ./root/usr/share/luci/menu.d/luci-app-5echarging.json $(1)/usr/share/luci/menu.d/luci-app-5echarging.json

	$(INSTALL_DIR) $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DATA) ./root/usr/share/rpcd/acl.d/luci-app-5echarging.json $(1)/usr/share/rpcd/acl.d/luci-app-5echarging.json
endef

define Package/$(PKG_NAME)/conffiles
/etc/config/5echarging
endef

define Package/$(PKG_NAME)/postinst
#!/bin/sh
[ -n "$$IPKG_INSTROOT" ] && exit 0

OLD_DB="/var/lib/echarging/echarging.db"
LEGACY_DB="/etc/echarging/echarging.db"
	OLD_DEFAULT_DB="/etc/5echarging/5echarging.db"
	NEW_DB="/etc/5echarging/5echarging.bbolt"

mkdir -p /etc/5echarging

if command -v uci >/dev/null 2>&1; then
	current_db="$$(uci -q get 5echarging.global.db_path)"
	if [ -z "$$current_db" ] || [ "$$current_db" = "$$OLD_DB" ] || [ "$$current_db" = "$$LEGACY_DB" ] || [ "$$current_db" = "$$OLD_DEFAULT_DB" ]; then
		uci set 5echarging.global.db_path="$$NEW_DB"
		uci commit 5echarging
	fi
fi

exit 0
endef

$(eval $(call BuildPackage,$(PKG_NAME)))
