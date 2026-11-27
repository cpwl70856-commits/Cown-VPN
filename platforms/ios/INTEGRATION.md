# iOS / iPadOS 集成指南（约 30 分钟上手）

目标：持有 **Apple Developer** 帐号的人，按本文在 Xcode 里搭出可在真机调试的 Packet Tunnel 客户端。

> 本仓库**不能**提供「免签名即可运行」的完整 `.xcodeproj`。Network Extension、App Group、Provisioning Profile 必须由你的开发者帐号签发。

---

## 你需要准备

| 项目 | 说明 |
|------|------|
| Mac + Xcode 15+ | gomobile / 真机调试需要 |
| Apple Developer Program | Network Extension 能力 |
| Go 1.22+ | 编译 `pkg/wgmobile` |
| 真机（推荐） | 模拟器对 NE/VPN 支持有限 |

---

## 总览架构

```
┌─────────────────────┐     App Group      ┌──────────────────────────┐
│  MyWGClient (SwiftUI)│ ───────────────► │ PacketTunnel (NE 扩展)   │
│  导入配置 / 连接按钮  │   wg0.conf        │ PacketTunnelProvider.swift│
└─────────────────────┘                    │   └─ WGMobile.xcframework │
                                           │       (gomobile bind)     │
                                           └──────────────────────────┘
```

桌面版 Fyne **不能**直接跑在 iOS 上；iOS 必须走 **Network Extension**。

---

## 步骤 1 — 用 gomobile 生成 XCFramework

在仓库根目录（Mac）：

```bash
# 1) 安装 gomobile
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
gomobile init

# 2) 拉取依赖
go mod tidy

# 3) 绑定 pkg/wgmobile → XCFramework
mkdir -p platforms/ios/build
gomobile bind -target=ios,iossimulator \
  -o platforms/ios/build/WGMobile.xcframework \
  github.com/user/mywgclient/pkg/wgmobile
```

导出的主要符号（Swift 侧名称可能带 `Wgmobile` 前缀）：

| Go API | 用途 |
|--------|------|
| `ParseConf(text)` | 返回 NetworkHints JSON（地址/DNS/AllowedIPs/Endpoint） |
| `BuildUAPI(text)` | 生成 wireguard-go `IpcSet` 字符串 |
| `Start` / `Stop` | 用户态隧道控制（桌面/实验；NE 内需配合 packet 桥接） |
| `Status` / `StatsJSON` | 状态与 RX/TX/握手 |

### 备选：c-archive

```bash
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
  go build -buildmode=c-archive \
  -o platforms/ios/build/libwgmobile.a \
  ./pkg/wgmobile
```

再在扩展里用 Bridging Header 调用。**gomobile XCFramework 对 Swift 更省事。**

### 生产建议

正式上架更稳妥的路径是官方 [wireguard-apple / WireGuardKit](https://git.zx2c4.com/wireguard-apple/)。  
`pkg/wgmobile` 适合：配置解析、UAPI 生成、状态 JSON，以及早期联调。完整数据面仍需把 `NEPacketTunnelFlow` 与 WireGuard 引擎桥接（见步骤 3）。

---

## 步骤 2 — 在 Xcode 创建工程与 Network Extension

1. Xcode → **File → New → Project → App**（SwiftUI，iOS）。
   - Product Name: `MyWGClient`
   - Bundle ID: `com.user.mywgclient`（可改，全文保持一致）
2. **File → New → Target → Network Extension → Packet Tunnel Provider**
   - Product Name: `PacketTunnel`
   - Bundle ID: `com.user.mywgclient.PacketTunnel`
3. 用本仓库文件替换/拷贝：
   - `platforms/ios/MyWGClientApp/*` → 主 App
   - `platforms/ios/PacketTunnel/*` → 扩展
4. 主 App 与扩展的 **Info.plist / entitlements** 参考同目录示例文件。

扩展 Info.plist 关键键：

```xml
<key>NSExtension</key>
<dict>
  <key>NSExtensionPointIdentifier</key>
  <string>com.apple.networkextension.packet-tunnel</string>
  <key>NSExtensionPrincipalClass</key>
  <string>$(PRODUCT_MODULE_NAME).PacketTunnelProvider</string>
</dict>
```

---

## 步骤 3 — 添加 Framework 并调用

1. 将 `WGMobile.xcframework` 拖入 Xcode。
2. 在 **PacketTunnel** target → *Frameworks, Libraries, and Embedded Content* 中添加，并设为 **Embed & Sign**。
3. 主 App 若也调用 `ParseConf`，同样 Link（扩展进程与 App 进程分离，扩展必须各自嵌入一份）。
4. `PacketTunnelProvider.swift` 已用 `#if canImport(WGMobile)` 调用：
   - `WgmobileParseConf` → 填 `NEPacketTunnelNetworkSettings`
   - `WgmobileStart` / `WgmobileStop` → 启停（实验）
   - `WgmobileStatsJSON` → `handleAppMessage` 回传

**数据包桥接（重要）**：iOS 不能像桌面那样 `tun.CreateTUN`。你需要：

1. `packetFlow.readPackets` 读到明文 IP 包  
2. 交给 WireGuard 引擎加密并经 UDP 发出  
3. 收密文 → 解密 → `packetFlow.writePackets`

WireGuardKit 已实现该循环；若坚持纯 Go，需自定义与 `NEPacketTunnelFlow` 对接的 TUN 适配层（工作量显著大于复制本模板）。

---

## 步骤 4 — 签名、App Group、NE Entitlements

在 [Apple Developer](https://developer.apple.com) → Identifiers：

1. 创建 App ID：`com.user.mywgclient`  
   - Capabilities: **Network Extensions**、**App Groups**
2. 创建 App ID：`com.user.mywgclient.PacketTunnel`  
   - 同上
3. 创建 App Group：`group.com.user.mywgclient`，并挂到两个 App ID
4. 为两个 ID 创建 Development Provisioning Profile，下载后在 Xcode 选择你的 Team

Xcode → 各 target → **Signing & Capabilities**：

- 勾选 **Network Extensions** → Packet Tunnel  
- 勾选 **App Groups** → `group.com.user.mywgclient`  
- 主 App entitlements 见 `MyWGClient.entitlements`  
- 扩展 entitlements 见 `PacketTunnel.entitlements`

首次连接时系统会弹出 **「允许 VPN 配置」** 对话框，用户必须点允许。

---

## 步骤 5 — 真机调试笔记

1. 用数据线连接 iPhone/iPad，信任电脑与开发者 App。
2. 方案选真机，先只跑主 App，确认能保存 conf 到 App Group。
3. 点「连接」：
   - 设置 → 通用 → VPN 与设备管理：应出现 `mywgclient`
   - 若扩展崩溃：Xcode → Debug → Attach to Process → `PacketTunnel`
4. 查看扩展日志：Console.app 过滤子系统 `com.user.mywgclient`
5. 常见失败：
   - `permission denied` / 无法保存 VPN：签名或 NE entitlement 缺失  
   - App Group 读不到 conf：Group ID 不一致或未加入两个 target  
   - 扩展秒退：未 Embed XCFramework，或 arm64 slice 不对  
   - 能连上但无流量：尚未实现 packet 桥接（骨架仅设置了 Network Settings）

模拟器：部分 iOS 版本对 Personal VPN / Packet Tunnel 支持不完整，**请优先真机**。

---

## 快速检查清单

- [ ] `gomobile bind` 产出 `WGMobile.xcframework`
- [ ] 主 App + PacketTunnel 两个 target
- [ ] 两边 Signing Team 相同，NE + App Group 已开
- [ ] Bundle ID / Group ID 与 Swift 常量一致
- [ ] 扩展 Embed 了 XCFramework
- [ ] 真机允许 VPN 配置
- [ ] （生产）换用 WireGuardKit 或完成 packet 桥接

---

## 无法在本仓库自动解决的限制

| 限制 | 原因 |
|------|------|
| 代码签名 / Provisioning | 必须用你的 Apple 帐号 |
| App Store 审核 | VPN 应用有额外合规要求 |
| Network Extension 权限 | 开发者后台开通 + entitlements |
| gomobile 工具链 | 需在 Mac + Xcode 上执行 |
| 完整可运行 Xcode 工程 | 含签名与 Team 信息，无法通用分发 |
| 纯 Go/Fyne 上架 iOS | 系统禁止普通 App 创建 TUN |
| 摄像头扫码 | 模板未含 AVFoundation；可后续加 |

按本文做完签名与 Framework 链接后，骨架即可在真机上弹出 VPN 许可并启动扩展；打通真实加密流量还需完成步骤 3 的 packet 桥接或接入 WireGuardKit。
