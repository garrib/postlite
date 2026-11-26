#!/bin/sh
# scripts/install.sh
# Postlite 一键安装：curl 下载不打 quarantine 标记，拷贝后主动剥离 xattr，
# 因此不会触发 Gatekeeper「Apple 无法验证」提示（docs/design.md §5）。
# 用法：
#   curl -fsSL https://raw.githubusercontent.com/garrib/postlite/main/scripts/install.sh | sh
#   或：sh install.sh [版本号]，中国大陆用户先 export https_proxy=http://127.0.0.1:7890
set -e

REPO="garrib/postlite"
APP_NAME="Postlite"
DEST="/Applications"

say() { printf '%s\n' "$*"; }

# 1. 取最新版 DMG 下载地址
VER="$1"
if [ -z "$VER" ] || [ "$VER" = "latest" ]; then
    URL="https://github.com/$REPO/releases/latest"
else
    URL="https://github.com/$REPO/releases/download/v$VER/Postlite-$VER-macos-universal.dmg"
fi
if [ -z "$VER" ] || [ "$VER" = "latest" ]; then
    say "→ 查询最新版本…"
    ASSET=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
        | sed -n 's/.*"browser_download_url": "\([^"]*Postlite-[^"]*\.dmg\)".*/\1/p' | head -1)
    [ -n "$ASSET" ] || { say "✗ 未找到最新 DMG 下载地址"; exit 1; }
else
    ASSET="https://github.com/$REPO/releases/download/v$VER/Postlite-$VER-macos-universal.dmg"
fi
say "→ 下载 $ASSET"

# 2. 下载（curl 不打 quarantine）
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
curl -fL --progress-bar -o "$TMP/Postlite.dmg" "$ASSET"

# 3. 挂载 → 拷贝 → 剥离 quarantine（双保险）
say "→ 挂载并安装到 $DEST"
MNT="$TMP/mnt"
mkdir -p "$MNT"
hdiutil attach -quiet -nobrowse -mountpoint "$MNT" "$TMP/Postlite.dmg"
[ -d "$MNT/$APP_NAME.app" ] || { say "✗ DMG 内未找到 $APP_NAME.app"; hdiutil detach -quiet "$MNT"; exit 1; }
rm -rf "$DEST/$APP_NAME.app"
cp -R "$MNT/$APP_NAME.app" "$DEST/"
xattr -dr com.apple.quarantine "$DEST/$APP_NAME.app" 2>/dev/null || true
hdiutil detach -quiet "$MNT"

say "✔ 已安装到 $DEST/$APP_NAME.app —— 直接打开即可，无需右键绕过"
