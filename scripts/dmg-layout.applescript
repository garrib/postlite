-- scripts/dmg-layout.applescript
-- 设置 Postlite DMG 卷的窗口布局：图标视图、隐藏工具栏、背景图、双图标定位。
-- 需要系统的「自动化 → Finder」授权；被拒绝时 make release 会降级为默认布局（|| true）。
tell application "Finder"
	tell disk "Postlite"
		open
		set current view of container window to icon view
		set toolbar visible of container window to false
		set statusbar visible of container window to false
		set the bounds of container window to {200, 120, 860, 540}
		set theViewOptions to the icon view options of container window
		set arrangement of theViewOptions to not arranged
		set icon size of theViewOptions to 100
		set background picture of theViewOptions to file ".background:bg.png"
		set position of item "Postlite.app" of container window to {170, 230}
		set position of item "Applications" of container window to {490, 230}
		close
		open
		update without registering applications
		delay 2
		close
	end tell
end tell
