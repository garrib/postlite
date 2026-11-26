VERSION ?= 0.2.5
# 注意：macOS APFS 大小写不敏感，bin/postlite 与 bin/Postlite 会撞名——Go 产物用 postlite-server。
BIN := bin/postlite-server
UI_BIN := bin/Postlite
ICON := build/AppIcon.icns
APP := bin/Postlite.app
DIST := frontend/dist
DMG := dist/Postlite-$(VERSION)-macos-universal.dmg
PROXY ?= http://127.0.0.1:7890

define APP_PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleExecutable</key><string>Postlite</string>
  <key>CFBundleIdentifier</key><string>xyz.garrib.postlite</string>
  <key>CFBundleName</key><string>Postlite</string>
  <key>CFBundleDisplayName</key><string>Postlite</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$(VERSION)</string>
  <key>CFBundleVersion</key><string>1</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>NSPrincipalClass</key><string>NSApplication</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
endef
export APP_PLIST

.PHONY: build build-dist build-ui icon app release test run clean

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

# macOS .app（双击即用）：Swift 壳 + Go server + 图标（当前架构）
app: build build-ui icon
	@rm -rf $(APP)
	@mkdir -p $(APP)/Contents/MacOS $(APP)/Contents/Resources
	@cp $(BIN) $(APP)/Contents/MacOS/postlite-server
	@cp $(UI_BIN) $(APP)/Contents/MacOS/Postlite
	@cp $(ICON) $(APP)/Contents/Resources/AppIcon.icns
	@echo "$$APP_PLIST" > $(APP)/Contents/Info.plist
	@codesign --force --deep -s - $(APP) >/dev/null 2>&1 || true
	@echo "✔ $(APP)（双击运行：独立窗口 + Dock 图标 + 原生菜单）"

# 发布包：universal（arm64 + x86_64）DMG 拖动安装窗口 + SHA256，用于 GitHub Release
# DMG 布局：Postlite.app 与 Applications 双图标 + 品牌背景（Finder 布局脚本失败则降级默认布局）
release: build-dist icon
	@mkdir -p dist
	go build -trimpath -ldflags "-s -w" -o bin/rel-server-arm64 .
	GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o bin/rel-server-x86_64 .
	lipo -create -output bin/rel-server bin/rel-server-arm64 bin/rel-server-x86_64
	# 本机 CLT swiftc 不认多 -arch，用 -target 分别编译再 lipo（PostliteApp.swift 无架构相关代码）
	swiftc -O -parse-as-library -target arm64-apple-macos12.0 -o bin/rel-shell-arm64 ui/PostliteApp.swift
	swiftc -O -parse-as-library -target x86_64-apple-macos12.0 -o bin/rel-shell-x86_64 ui/PostliteApp.swift
	lipo -create -output bin/rel-shell bin/rel-shell-arm64 bin/rel-shell-x86_64
	@rm -rf dist/Postlite.app
	@mkdir -p dist/Postlite.app/Contents/MacOS dist/Postlite.app/Contents/Resources
	@cp bin/rel-server dist/Postlite.app/Contents/MacOS/postlite-server
	@cp bin/rel-shell dist/Postlite.app/Contents/MacOS/Postlite
	@cp $(ICON) dist/Postlite.app/Contents/Resources/AppIcon.icns
	@echo "$$APP_PLIST" > dist/Postlite.app/Contents/Info.plist
	@codesign --force --deep -s - dist/Postlite.app >/dev/null 2>&1 || true
	# —— DMG（dmgbuild：拖动安装窗口，无需 Finder 自动化授权；构建期 venv 依赖）——
	swiftc -O -o build/make-dmg-bg scripts/make-dmg-bg.swift
	build/make-dmg-bg build/dmg-bg.png
	@rm -f $(DMG)
	@if [ ! -x build/venv/bin/dmgbuild ]; then \
		python3 -m venv build/venv && \
		https_proxy=$(PROXY) http_proxy=$(PROXY) build/venv/bin/pip -q install dmgbuild; \
	fi
	build/venv/bin/dmgbuild -s scripts/dmg-settings.py "Postlite" $(DMG)
	@cd dist && shasum -a 256 $(notdir $(DMG)) | tee $(notdir $(basename $(DMG)))-SHA256.txt
	@rm -rf dist/Postlite.app
	@echo "✔ $(DMG)（拖动安装窗口）"

# 本地起服务（CLI 形态，不自动开浏览器）
run: build
	POSTLITE_NO_OPEN=1 $(BIN)

clean:
	rm -rf bin dist build
	rm -rf $(DIST)
	mkdir -p $(DIST)
	touch $(DIST)/.gitkeep
