// scripts/make-dmg-bg.swift
// 生成 DMG 拖动安装窗口背景图（660x420，与 scripts/dmg-settings.py 的 window_rect 1:1）。
// 布局（窗口顶起算）：标题 56 · 副标题 100 · 图标中心 y=260（dmgbuild 图标定位 (140,210)/(420,210) size=100）
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

// 右向箭头：细线圆头，位于两图标中心连线上（AppKit y = 420-260 = 160），
// 避开左右图标（左图标右缘 240、右图标左缘 420）
let arrow = NSBezierPath()
arrow.lineWidth = 5
arrow.lineCapStyle = .round
arrow.lineJoinStyle = .round
arrow.move(to: NSPoint(x: 252, y: 160))
arrow.line(to: NSPoint(x: 398, y: 160))
brand.setStroke()
arrow.stroke()

let head = NSBezierPath()
head.lineWidth = 5
head.lineCapStyle = .round
head.lineJoinStyle = .round
head.move(to: NSPoint(x: 384, y: 174))
head.line(to: NSPoint(x: 400, y: 160))
head.line(to: NSPoint(x: 384, y: 146))
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
