# scripts/dmg-settings.py
# dmgbuild 配置：Postlite 拖动安装窗口（背景图 + 双图标定位 + 隐藏状态栏/工具栏）。
# 由 make release 调用：build/venv/bin/dmgbuild -s scripts/dmg-settings.py "Postlite" <out.dmg>

app_path = 'dist/Postlite.app'

# UDZO = 压缩只读镜像
format = 'UDZO'

files = [app_path]
symlinks = {'Applications': '/Applications'}

# 品牌背景（scripts/make-dmg-bg.swift 生成，660x420）
background = 'build/dmg-bg.png'

# WindowBounds 语义为 {{左, 下}, {宽, 高}}：窗口 660x420 与背景图 1:1，不出白边
window_rect = ((440, 240), (660, 420))

default_view = 'icon-view'
icon_size = 100
text_size = 13
show_status_bar = False
show_toolbar = False
show_tab_view = False
show_pathbar = False
show_sidebar = False

# 图标定位：dmgbuild 此值为图标「中心」坐标（实测渲染中心=给定值，非左上角）。
# 对称布局：左右各留 190pt，行中心 y=255（标题区下方居中）。
icon_locations = {
    'Postlite.app': (190, 255),
    'Applications': (470, 255),
}
