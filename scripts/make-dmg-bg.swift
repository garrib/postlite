// scripts/make-dmg-bg.swift
// 生成 DMG 拖动安装窗口背景图（660x420，与 scripts/dmg-settings.py 的 window_rect 1:1）。
// 设计只有顶部文字（标题 56 / 副标题 100，窗口顶起算）——刻意不画箭头：
// dmgbuild 的 icon_locations 是图标中心坐标，箭头跨坐标系对齐太脆弱，图标本身即拖装暗示。
// 构建：swiftc -O -o build/make-dmg-bg scripts/make-dmg-bg.swift
// 运行：build/make-dmg-bg <输出.png>
import AppKit

let out = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "build/dmg-bg.png"
let w = 660.0, h = 420.0

let img = NSImage(size: NSSize(width: w, height: h))
img.lockFocus()

// 底：白 → 浅灰渐变（浅色主题一致）
NSColor.white.setFill()
NSRect(x: 0, y: 0, width: w, height: h).fill()
NSGradient(colors: [
    NSColor(red: 1, green: 1, blue: 1, alpha: 1),
    NSColor(red: 0.945, green: 0.956, blue: 0.949, alpha: 1),
])?.draw(in: NSRect(x: 0, y: 0, width: w, height: h), angle: -90)

let brand = NSColor(red: 0.067, green: 0.165, blue: 0.145, alpha: 1) // #112A25
let para = NSMutableParagraphStyle()
para.alignment = .center

// 主标题（窗口顶 56pt 起）
("拖动 Postlite 到 Applications 完成安装" as NSString).draw(
    in: NSRect(x: 0, y: h - 56 - 36, width: w, height: 36),
    withAttributes: [
        .font: NSFont.systemFont(ofSize: 21, weight: .semibold),
        .foregroundColor: brand,
        .paragraphStyle: para,
    ]
)
// 副标题（紧跟标题下方，顺序：标题在上、副标题在下）
("免费 · 免登录 · 双击即用" as NSString).draw(
    in: NSRect(x: 0, y: h - 100 - 22, width: w, height: 22),
    withAttributes: [
        .font: NSFont.systemFont(ofSize: 13),
        .foregroundColor: NSColor(white: 0.45, alpha: 1),
        .paragraphStyle: para,
    ]
)

img.unlockFocus()

guard let tiff = img.tiffRepresentation,
      let rep = NSBitmapImageRep(data: tiff),
      let png = rep.representation(using: .png, properties: [:]) else {
    fputs("make-dmg-bg: 渲染失败\n", stderr)
    exit(1)
}
try! png.write(to: URL(fileURLWithPath: out))
print("dmg 背景 → \(out)")
