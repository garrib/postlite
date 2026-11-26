VERSION ?= 0.2.3
# 注意：macOS APFS 大小写不敏感，bin/postlite 与 bin/Postlite 会撞名——Go 产物用 postlite-server。
BIN := bin/postlite-server
UI_BIN := bin/Postlite
ICON := build/AppIcon.icns
APP := bin/Postlite.app
DIST := frontend/dist

.PHONY: build build-dist build-ui icon app test run clean

# 构建前端产物（node_modules 缺失才走 npm ci，日常增量直接 vite build）
build-dist:
	@test -d frontend/node_modules || (cd frontend && npm ci --no-audit --no-fund)
	cd frontend && npm run build
	@touch frontend/dist/.gitkeep   # vite build 清空 outDir 会删占位符；go:embed 需要目录非空

# Go 服务（CLI 形态：单独运行，自动开浏览器；也是 .app 内的 postlite-server）
build: build-dist
	go build -trimpath -ldflags "-s -w" -o $(BIN) .

# Swift 原生壳（单文件直编，不经 SPM；-parse-as-library 让 @main 生效）
build-ui:
	@mkdir -p bin
	swiftc -O -parse-as-library -o $(UI_BIN) ui/PostliteApp.swift

# 应用图标：CoreGraphics 生成 iconset → iconutil 转 icns
icon:
	@mkdir -p build/icon.iconset bin
	swiftc -O -o build/make-icon scripts/make-icon.swift
	build/make-icon build/icon.iconset
	iconutil -c icns -o $(ICON) build/icon.iconset

# 全量测试：Go + 前端
test:
	go test ./...
	cd frontend && npm test

# macOS .app（双击即用）：Swift 壳 + Go server + 图标
app: build build-ui icon
	@rm -rf $(APP)
	@mkdir -p $(APP)/Contents/MacOS $(APP)/Contents/Resources
	@cp $(BIN) $(APP)/Contents/MacOS/postlite-server
	@cp $(UI_BIN) $(APP)/Contents/MacOS/Postlite
	@cp $(ICON) $(APP)/Contents/Resources/AppIcon.icns
	@printf '%s\n' \
		'<?xml version="1.0" encoding="UTF-8"?>' \
		'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
		'<plist version="1.0"><dict>' \
		'  <key>CFBundleExecutable</key><string>Postlite</string>' \
		'  <key>CFBundleIdentifier</key><string>xyz.garrib.postlite</string>' \
		'  <key>CFBundleName</key><string>Postlite</string>' \
		'  <key>CFBundleDisplayName</key><string>Postlite</string>' \
		'  <key>CFBundlePackageType</key><string>APPL</string>' \
		'  <key>CFBundleShortVersionString</key><string>$(VERSION)</string>' \
		'  <key>CFBundleVersion</key><string>1</string>' \
		'  <key>CFBundleIconFile</key><string>AppIcon</string>' \
		'  <key>NSPrincipalClass</key><string>NSApplication</string>' \
		'  <key>LSMinimumSystemVersion</key><string>12.0</string>' \
		'  <key>NSHighResolutionCapable</key><true/>' \
		'</dict></plist>' > $(APP)/Contents/Info.plist
	@codesign --force --deep -s - $(APP) >/dev/null 2>&1 || true
	@echo "✔ $(APP)（双击运行：独立窗口 + Dock 图标 + 原生菜单）"

# 本地起服务（CLI 形态，不自动开浏览器）
run: build
	POSTLITE_NO_OPEN=1 $(BIN)

clean:
	rm -rf bin build
	rm -rf $(DIST)
	mkdir -p $(DIST)
	touch $(DIST)/.gitkeep
