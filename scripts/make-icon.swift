// scripts/make-icon.swift
// 生成 Postlite 应用图标（design.md §4.3：Big Sur squircle + Postman 橙渐变 + 白色纸飞机）。
// 构建：swiftc -O -o build/make-icon scripts/make-icon.swift
// 运行：build/make-icon <输出目录（iconset）>
import Foundation
import CoreGraphics
import ImageIO
import UniformTypeIdentifiers

let outDir = CommandLine.arguments.count > 1
    ? URL(fileURLWithPath: CommandLine.arguments[1])
    : URL(fileURLWithPath: "build/icon.iconset")
try? FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)

let entries: [(base: Int, scale: Int)] = [
    (16, 1), (16, 2), (32, 1), (32, 2),
    (128, 1), (128, 2), (256, 1), (256, 2),
    (512, 1), (512, 2),
]

func render(_ px: CGFloat) -> CGImage? {
    let size = Int(px)
    guard
        let space = CGColorSpace(name: CGColorSpace.sRGB),
        let ctx = CGContext(
            data: nil, width: size, height: size,
            bitsPerComponent: 8, bytesPerRow: 0, space: space,
            bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue
        )
    else { return nil }

    // 透明画布留 10% 边距；圆角 22.37%（macOS squircle 惯例）
    let inset = px * 0.10
    let rect = CGRect(x: inset, y: inset, width: px - inset * 2, height: px - inset * 2)
    let radius = rect.width * 0.2237

    ctx.saveGState()
    ctx.addPath(CGPath(roundedRect: rect, cornerWidth: radius, cornerHeight: radius, transform: nil))
    ctx.clip()

    // 底：品牌主色垂直渐变（#1E4A41 → #112A25，design.md §4.3）
    let colors = [
        CGColor(red: 0.118, green: 0.290, blue: 0.255, alpha: 1),
        CGColor(red: 0.067, green: 0.165, blue: 0.145, alpha: 1),
    ]
    if let grad = CGGradient(colorsSpace: space, colors: colors as CFArray, locations: [0, 1]) {
        ctx.drawLinearGradient(
            grad,
            start: CGPoint(x: rect.midX, y: rect.maxY),
            end: CGPoint(x: rect.midX, y: rect.minY),
            options: []
        )
    }

    // 白色纸飞机（设计坐标 y 从上往下，转成 CG 的 y-up）
    let s = rect.width
    func pt(_ x: CGFloat, _ y: CGFloat) -> CGPoint {
        CGPoint(x: rect.minX + x * s, y: rect.maxY - y * s)
    }
    ctx.setFillColor(CGColor(red: 1, green: 1, blue: 1, alpha: 1))
    ctx.move(to: pt(0.26, 0.54))
    ctx.addLine(to: pt(0.82, 0.24))
    ctx.addLine(to: pt(0.60, 0.80))
    ctx.closePath()
    ctx.fillPath()
    // 机身折角（深一号主色，压出层次）
    ctx.setFillColor(CGColor(red: 0.051, green: 0.125, blue: 0.106, alpha: 1))
    ctx.move(to: pt(0.47, 0.63))
    ctx.addLine(to: pt(0.60, 0.80))
    ctx.addLine(to: pt(0.38, 0.75))
    ctx.closePath()
    ctx.fillPath()

    ctx.restoreGState()
    return ctx.makeImage()
}

for e in entries {
    let px = CGFloat(e.base * e.scale)
    guard let img = render(px) else { continue }
    let name = e.scale == 1 ? "icon_\(e.base)x\(e.base).png" : "icon_\(e.base)x\(e.base)@2x.png"
    let url = outDir.appendingPathComponent(name)
    guard let dest = CGImageDestinationCreateWithURL(url as CFURL, UTType.png.identifier as CFString, 1, nil) else { continue }
    CGImageDestinationAddImage(dest, img, nil)
    CGImageDestinationFinalize(dest)
}
print("iconset 已生成 → \(outDir.path)")
