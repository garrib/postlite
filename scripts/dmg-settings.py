# scripts/dmg-settings.py
# dmgbuild 配置：Postlite 拖动安装窗口（背景图 + 双图标定位 + 隐藏工具栏）。
# 由 make release 调用：build/venv/bin/dmgbuild -s scripts/dmg-settings.py -v Postlite <out.dmg>

app_path = 'dist/Postlite.app'

# UDZO = 压缩只读镜像
format = 'UDZO'

files = [app_path]
symlinks = {'Applications': '/Applications'}

# 品牌背景（scripts/make-dmg-bg.swift 生成，660x420）
background = 'build/dmg-bg.png'

# 窗口与图标布局（scripts/dmg-layout.applescript 同款坐标）
window_rect = ((200, 120), (860, 540))
default_view = 'icon-view'
icon_size = 100
icon_locations = {
    'Postlite.app': (170, 230),
    'Applications': (490, 230),
}
