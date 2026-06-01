#!/bin/bash
set -e

# 默认在本项目根目录查找名为 openwrt-sdk-* 的目录
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
PKG_NAME="luci-app-5echarging"
PKG_VERSION="1.0.1"
PKG_RELEASE="1"
PKG_DIR="$SCRIPT_DIR/$PKG_NAME"

SDK_DIR=""
if [ -n "$1" ]; then
    SDK_DIR="$1"
else
    SDK_DIR=$(find "$SCRIPT_DIR" -maxdepth 1 -type d -name "openwrt-sdk-*" | head -n 1)
fi

if [ -z "$SDK_DIR" ] || [ ! -d "$SDK_DIR" ]; then
    echo "错误: 未找到 OpenWrt SDK 目录。"
    echo "请将 OpenWrt SDK 解压到当前目录下，或者通过参数指定路径。"
    echo "用法: $0 [/path/to/openwrt-sdk]"
    exit 1
fi

SDK_DIR=$(realpath "$SDK_DIR")
echo "=> 使用 OpenWrt SDK 路径: $SDK_DIR"

PKG_ARCH=$(find "$SDK_DIR/bin/packages" -mindepth 1 -maxdepth 1 -type d -printf "%f\n" 2>/dev/null | head -n 1)
if [ -z "$PKG_ARCH" ]; then
    PKG_ARCH=$(find "$SDK_DIR/staging_dir" -maxdepth 1 -type d -name "target-*" | head -n 1 | xargs -r basename | sed 's/target-//g' | cut -d'_' -f1,2)
fi
if [ -z "$PKG_ARCH" ]; then
    PKG_ARCH="all"
fi
echo "=> 检测到 SDK 包输出架构目录: $PKG_ARCH"

ORIG_DIR="$SCRIPT_DIR"

IPKG_BUILD="$SDK_DIR/scripts/ipkg-build"
if [ ! -x "$IPKG_BUILD" ]; then
    echo "错误: SDK 中未找到 scripts/ipkg-build。"
    exit 1
fi

echo "=> 准备 ipk 文件结构..."
BUILD_DIR="$ORIG_DIR/build/ipkg"
STAGE_DIR="$BUILD_DIR/$PKG_NAME"
OUT_DIR="$SDK_DIR/bin/packages/$PKG_ARCH/base"

rm -rf "$ORIG_DIR/build"
rm -f "$ORIG_DIR/${PKG_NAME}_"*.ipk
mkdir -p "$OUT_DIR"
rm -f "$OUT_DIR/${PKG_NAME}_"*.ipk

mkdir -p "$STAGE_DIR/CONTROL"
mkdir -p "$STAGE_DIR/usr/bin"
mkdir -p "$STAGE_DIR/etc/config"
mkdir -p "$STAGE_DIR/etc/init.d"
mkdir -p "$STAGE_DIR/usr/lib/lua/luci/controller"
mkdir -p "$STAGE_DIR/usr/lib/lua/luci/model/cbi"
mkdir -p "$STAGE_DIR/usr/lib/lua/luci/view/5echarging"
mkdir -p "$STAGE_DIR/usr/share/luci/menu.d"
mkdir -p "$STAGE_DIR/usr/share/rpcd/acl.d"

install -m 0755 "$PKG_DIR/files/usr/bin/5echarging-uci2json" "$STAGE_DIR/usr/bin/5echarging-uci2json"
install -m 0644 "$PKG_DIR/files/etc/config/5echarging" "$STAGE_DIR/etc/config/5echarging"
install -m 0755 "$PKG_DIR/files/etc/init.d/5echarging" "$STAGE_DIR/etc/init.d/5echarging"
install -m 0644 "$PKG_DIR/luasrc/controller/5echarging.lua" "$STAGE_DIR/usr/lib/lua/luci/controller/5echarging.lua"
install -m 0644 "$PKG_DIR/luasrc/model/cbi/5echarging.lua" "$STAGE_DIR/usr/lib/lua/luci/model/cbi/5echarging.lua"
install -m 0644 "$PKG_DIR/luasrc/view/5echarging/status.htm" "$STAGE_DIR/usr/lib/lua/luci/view/5echarging/status.htm"
install -m 0644 "$PKG_DIR/root/usr/share/luci/menu.d/luci-app-5echarging.json" "$STAGE_DIR/usr/share/luci/menu.d/luci-app-5echarging.json"
install -m 0644 "$PKG_DIR/root/usr/share/rpcd/acl.d/luci-app-5echarging.json" "$STAGE_DIR/usr/share/rpcd/acl.d/luci-app-5echarging.json"

cat > "$STAGE_DIR/CONTROL/control" <<EOF
Package: $PKG_NAME
Version: $PKG_VERSION-r$PKG_RELEASE
Depends: libc, luci-base, rpcd, uclient-fetch, ca-bundle
Source: 
SourceName: $PKG_NAME
Section: luci
Maintainer: Quiet
Architecture: all
Installed-Size: 0
Description: 宿舍电费自动查询、预测与低余额提醒
EOF

cat > "$STAGE_DIR/CONTROL/conffiles" <<EOF
/etc/config/5echarging
EOF

cat > "$STAGE_DIR/CONTROL/postinst" <<'EOF'
#!/bin/sh
[ -n "$IPKG_INSTROOT" ] && exit 0

OLD_DB="/var/lib/echarging/echarging.db"
LEGACY_DB="/etc/echarging/echarging.db"
OLD_DEFAULT_DB="/etc/5echarging/5echarging.db"
NEW_DB="/etc/5echarging/5echarging.bbolt"

mkdir -p /etc/5echarging

if command -v uci >/dev/null 2>&1; then
	current_db="$(uci -q get 5echarging.global.db_path)"
	if [ -z "$current_db" ] || [ "$current_db" = "$OLD_DB" ] || [ "$current_db" = "$LEGACY_DB" ] || [ "$current_db" = "$OLD_DEFAULT_DB" ]; then
		uci set 5echarging.global.db_path="$NEW_DB"
		uci commit 5echarging
	fi
fi

exit 0
EOF
chmod 0755 "$STAGE_DIR/CONTROL/postinst"

echo "=> 使用 SDK ipkg-build 打包 ipk..."
LC_ALL=C LANG=C PATH="$SDK_DIR/staging_dir/host/bin:$PATH" TOPDIR="$SDK_DIR" "$IPKG_BUILD" "$STAGE_DIR" "$OUT_DIR"

IPK_FILE=$(find "$OUT_DIR" -maxdepth 1 -name "${PKG_NAME}_${PKG_VERSION}-r${PKG_RELEASE}_all.ipk" | head -n 1)
if [ -z "$IPK_FILE" ]; then
    IPK_FILE=$(find "$OUT_DIR" -maxdepth 1 -name "${PKG_NAME}_*.ipk" | head -n 1)
fi

if [ -z "$IPK_FILE" ]; then
    echo "错误: 未找到生成的 ipk 文件。"
    exit 1
fi

cp "$IPK_FILE" "$ORIG_DIR/"
rm -rf "$ORIG_DIR/build"

echo
echo "================================================="
echo "编译成功，成品 ipk 文件已复制到项目根目录："
echo "$ORIG_DIR/$(basename "$IPK_FILE")"
echo "================================================="
