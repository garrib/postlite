// ui/PostliteApp.swift
// Postlite 原生壳：WKWebView 窗口 + postlite-server（Go）子进程。
// 架构与约束见 docs/design.md §4（双进程模型 / 父进程监控 / 原生下载桥）。
// 构建：swiftc -O -o Postlite ui/PostliteApp.swift（单文件直编，不经 SPM）。

import Cocoa
import WebKit

@main
final class AppDelegate: NSObject, NSApplicationDelegate, WKNavigationDelegate, WKScriptMessageHandler {

    private var window: NSWindow?
    private var webView: WKWebView?
    private var server: Process?
    private var terminating = false

    static func main() {
        let app = NSApplication.shared
        let delegate = AppDelegate()
        app.delegate = delegate
        app.setActivationPolicy(.regular)
        app.run()
    }

    // MARK: - 生命周期

    func applicationDidFinishLaunching(_ notification: Notification) {
        buildMenu()
        startServer()
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }

    func applicationWillTerminate(_ notification: Notification) {
        terminating = true
        if let p = server, p.isRunning { p.terminate() }
    }

    // MARK: - server 子进程

    private func startServer() {
        let exe = Bundle.main.bundleURL.appendingPathComponent("Contents/MacOS/postlite-server")
        guard FileManager.default.fileExists(atPath: exe.path) else {
            failLaunch("未找到 postlite-server（Contents/MacOS/postlite-server）。请重新执行 make app 打包。")
            return
        }
        let p = Process()
        p.executableURL = exe
        var env = ProcessInfo.processInfo.environment
        env["POSTLITE_NO_OPEN"] = "1"
        env["POSTLITE_WATCH_PARENT"] = "1"
        p.environment = env

        // Go 的 log 输出走 stderr；stdout/stderr 合并读，从中解析实际监听地址。
        let pipe = Pipe()
        p.standardOutput = pipe
        p.standardError = pipe
        pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            guard !data.isEmpty else { handle.readabilityHandler = nil; return }
            let text = String(data: data, encoding: .utf8) ?? ""
            if let url = Self.firstURL(in: text), let self = self {
                DispatchQueue.main.async { self.serverReady(url: url) }
            }
        }
        p.terminationHandler = { [weak self] process in
            DispatchQueue.main.async {
                guard let self = self, !self.terminating else { return }
                self.failLaunch("postlite-server 意外退出（status \(process.terminationStatus)），应用即将关闭。")
            }
        }
        do {
            try p.run()
            server = p
        } catch {
            failLaunch("postlite-server 启动失败：\(error.localizedDescription)")
        }
    }

    /// 从子进程日志解析实际监听地址（端口被占自动 +1 后以日志为准）。
    private static func firstURL(in text: String) -> URL? {
        guard let r = text.range(of: #"http://127\.0\.0\.1:\d+"#, options: .regularExpression) else { return nil }
        return URL(string: String(text[r]))
    }

    private func serverReady(url: URL) {
        guard window == nil else { return } // 只处理首个日志行
        buildWindow(url: url)
    }

    // MARK: - 窗口与 Web UI

    private func buildWindow(url: URL) {
        let config = WKWebViewConfiguration()
        // 强引用 self：AppDelegate 与应用同生命周期，无泄漏风险。
        config.userContentController.add(self, name: "postlite")

        let wb = WKWebView(frame: NSRect(x: 0, y: 0, width: 1220, height: 780), configuration: config)
        wb.navigationDelegate = self
        wb.allowsMagnification = false

        let content = NSView(frame: NSRect(x: 0, y: 0, width: 1220, height: 780))
        content.addSubview(wb)
        wb.translatesAutoresizingMaskIntoConstraints = false
        NSLayoutConstraint.activate([
            wb.topAnchor.constraint(equalTo: content.topAnchor),
            wb.bottomAnchor.constraint(equalTo: content.bottomAnchor),
            wb.leadingAnchor.constraint(equalTo: content.leadingAnchor),
            wb.trailingAnchor.constraint(equalTo: content.trailingAnchor),
        ])

        let w = NSWindow(
            contentRect: content.frame,
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered,
            defer: false
        )
        w.title = "Postlite"
        w.minSize = NSSize(width: 960, height: 620)
        w.contentView = content
        w.center()
        w.makeKeyAndOrderFront(nil)
        window = w
        webView = wb
        wb.load(URLRequest(url: url))
    }

    // MARK: - WKNavigationDelegate：外链交给系统浏览器，窗口只留在本机服务

    func webView(
        _ webView: WKWebView,
        decidePolicyFor navigationAction: WKNavigationAction,
        decisionHandler: @escaping (WKNavigationActionPolicy) -> Void
    ) {
        if navigationAction.navigationType == .linkActivated,
           let url = navigationAction.request.url,
           let host = url.host, host != "127.0.0.1" {
            NSWorkspace.shared.open(url)
            decisionHandler(.cancel)
            return
        }
        decisionHandler(.allow)
    }

    // MARK: - 原生桥（window.webkit.messageHandlers.postlite）

    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        guard let body = message.body as? [String: Any], let name = body["name"] as? String else { return }
        if name == "download",
           let base64 = body["base64"] as? String,
           let filename = body["filename"] as? String {
            saveFile(base64: base64, filename: filename)
        }
    }

    private func saveFile(base64: String, filename: String) {
        guard let data = Data(base64Encoded: base64), let w = window else { return }
        let panel = NSSavePanel()
        panel.nameFieldStringValue = filename
        panel.canCreateDirectories = true
        panel.beginSheetModal(for: w) { resp in
            if resp == .OK, let url = panel.url {
                try? data.write(to: url)
            }
        }
    }

    // MARK: - 菜单（Edit 菜单是 WKWebView 文本编辑快捷键生效的前提）

    private func buildMenu() {
        let mainMenu = NSMenu()

        let appItem = NSMenuItem()
        let appMenu = NSMenu()
        appMenu.addItem(withTitle: "关于 Postlite", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
        appMenu.addItem(.separator())
        appMenu.addItem(withTitle: "退出 Postlite", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        appItem.submenu = appMenu
        mainMenu.addItem(appItem)

        let editItem = NSMenuItem(title: "编辑", action: nil, keyEquivalent: "")
        let editMenu = NSMenu(title: "编辑")
        editMenu.addItem(withTitle: "撤销", action: Selector(("undo:")), keyEquivalent: "z")
        editMenu.addItem(withTitle: "重做", action: Selector(("redo:")), keyEquivalent: "Z")
        editMenu.addItem(.separator())
        editMenu.addItem(withTitle: "剪切", action: Selector(("cut:")), keyEquivalent: "x")
        editMenu.addItem(withTitle: "复制", action: Selector(("copy:")), keyEquivalent: "c")
        editMenu.addItem(withTitle: "粘贴", action: Selector(("paste:")), keyEquivalent: "v")
        editMenu.addItem(withTitle: "全选", action: Selector(("selectAll:")), keyEquivalent: "a")
        editItem.submenu = editMenu
        mainMenu.addItem(editItem)

        let winItem = NSMenuItem(title: "窗口", action: nil, keyEquivalent: "")
        let winMenu = NSMenu(title: "窗口")
        winMenu.addItem(withTitle: "最小化", action: Selector(("performMiniaturize:")), keyEquivalent: "m")
        winMenu.addItem(withTitle: "缩放", action: Selector(("performZoom:")), keyEquivalent: "")
        winMenu.addItem(.separator())
        winMenu.addItem(withTitle: "前置全部窗口", action: Selector(("arrangeInFront:")), keyEquivalent: "")
        winItem.submenu = winMenu
        mainMenu.addItem(winItem)

        NSApp.mainMenu = mainMenu
    }

    private func failLaunch(_ message: String) {
        let alert = NSAlert()
        alert.messageText = "Postlite 无法启动"
        alert.informativeText = message
        alert.runModal()
        NSApp.terminate(nil)
    }
}
