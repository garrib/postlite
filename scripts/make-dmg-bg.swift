// scripts/make-dmg-bg.swift
// 生成 DMG 拖动安装窗口背景图（660x420，brand 主色文字 + 右向箭头）。
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

// 主文案 + 副文案（窗口顶部居中）
("拖动 Postlite 到 Applications，完成安装" as NSString).draw(
    in: NSRect(x: 0, y: h - 110, width: w, height: 40),
    withAttributes: [
        .font: NSFont.systemFont(ofSize: 22, weight: .semibold),
        .foregroundColor: brand,
        .paragraphStyle: para,
    ]
)
("免费 · 免登录 · 双击即用" as NSString).draw(
    in: NSRect(x: 0, y: h - 80, width: w, height: 26),
    withAttributes: [
        .font: NSFont.systemFont(ofSize: 14),
        .foregroundColor: NSColor(white: 0.42, alpha: 1),
        .paragraphStyle: para,
    ]
)

// 右向箭头（app 图标与 Applications 图标之间，窗口 y≈280 处）
let line = NSBezierPath()
line.lineWidth = 8
line.lineCapStyle = .round
line.move(to: NSPoint(x: 300, y: 140))
line.line(to: NSPoint(x: 440, y: 140))
brand.setStroke()
line.stroke()

let head = NSBezierPath()
head.lineWidth = 8
head.lineCapStyle = .round
head.lineJoinStyle = .round
head.move(to: NSPoint(x: 415, y: 165))
head.line(to: NSPoint(x: 445, y: 140))
head.line(to: NSPoint(x: 415, y: 115))
head.stroke()

img.unlockFocus()

guard let tiff = img.tiffRepresentation,
      let rep = NSBitmapImageRep(data: tiff),
      let png = rep.representation(using: .png, properties: [:]) else {
    fputs("make-dmg-bg: 渲染失败\n", stderr)
    exit(1)
}
try! png.write(to: URL(fileURLWithPath: out))
print("dmg 背景 → \(out)")
