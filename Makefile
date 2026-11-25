VERSION ?= 0.1.0
BIN := bin/postlite
APP := bin/Postlite.app
DIST := frontend/dist

.PHONY: build build-dist test app run clean

# 构建前端产物（node_modules 缺失才走 npm ci，日常增量直接 vite build）
build-dist:
	@test -d frontend/node_modules || (cd frontend && npm ci --no-audit --no-fund)
	cd frontend && npm run build
	@touch frontend/dist/.gitkeep   # vite build 清空 outDir 会删占位符；go:embed 需要目录非空

# 单二进制：前端产物经 go:embed 内嵌
build: build-dist
	go build -trimpath -ldflags "-s -w" -o $(BIN) .

# 全量测试：Go + 前端
test:
	go test ./...
	cd frontend && npm test

# macOS .app（双击即用）
app: build
	@rm -rf $(APP)
	@mkdir -p $(APP)/Contents/MacOS
	@cp $(BIN) $(APP)/Contents/MacOS/postlite
	@printf '%s\n' \
		'<?xml version="1.0" encoding="UTF-8"?>' \
		'<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">' \
		'<plist version="1.0"><dict>' \
		'  <key>CFBundleExecutable</key><string>postlite</string>' \
		'  <key>CFBundleIdentifier</key><string>xyz.garrib.postlite</string>' \
		'  <key>CFBundleName</key><string>Postlite</string>' \
		'  <key>CFBundleDisplayName</key><string>Postlite</string>' \
		'  <key>CFBundlePackageType</key><string>APPL</string>' \
		'  <key>CFBundleShortVersionString</key><string>$(VERSION)</string>' \
		'  <key>CFBundleVersion</key><string>1</string>' \
		'  <key>LSMinimumSystemVersion</key><string>11.0</string>' \
		'  <key>NSHighResolutionCapable</key><true/>' \
		'  <key>LSUIElement</key><true/>' \
		'</dict></plist>' > $(APP)/Contents/Info.plist
	@codesign --force -s - $(APP) >/dev/null 2>&1 || true
	@echo "✔ $(APP)（双击运行，自动打开浏览器）"

# 本地起服务（不自动开浏览器）
run: build
	POSTLITE_NO_OPEN=1 $(BIN)

clean:
	rm -rf bin
	rm -rf $(DIST)
	mkdir -p $(DIST)
	touch $(DIST)/.gitkeep
