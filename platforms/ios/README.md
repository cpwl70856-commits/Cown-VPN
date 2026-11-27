# iOS / iPadOS 模板

面向 **Network Extension（Packet Tunnel）** 的示例代码与说明。桌面版 Fyne 客户端不能直接在 iOS 上创建 TUN。

## 目录

```
platforms/ios/
  INTEGRATION.md                 ← 详细集成步骤（请从这里开始）
  README.md                      ← 本文件
  MyWGClientApp/                 ← SwiftUI 主 App 示例
    MyWGClientApp.swift
    ContentView.swift
    VPNManager.swift
    Info.plist
    MyWGClient.entitlements
  PacketTunnel/                  ← NE 扩展示例
    PacketTunnelProvider.swift
    Info.plist
    PacketTunnel.entitlements
```

Go 侧 gomobile API：`pkg/wgmobile`（`ParseConf` / `BuildUAPI` / `Start` / `Stop` / `Status` / `StatsJSON`）。

## 快速开始

请阅读 **[INTEGRATION.md](./INTEGRATION.md)**，按顺序完成：

1. `gomobile bind` 生成 `WGMobile.xcframework`
2. Xcode 创建 App + Packet Tunnel 扩展
3. 嵌入 Framework 并调用
4. 配置签名、App Group、NE entitlements
5. 真机调试

## 限制摘要

- 必须使用 Network Extension，不能用纯 Go/Fyne
- 需要 Apple Developer 帐号与正确签名
- 本仓库不附带可直接运行的已签名 Xcode 工程
- 骨架默认未完成完整 packet 桥接；生产环境推荐 [WireGuardKit](https://git.zx2c4.com/wireguard-apple/)
