import SwiftUI

/// 简易 SwiftUI 主界面：状态、导入配置、连接 / 断开。
struct ContentView: View {
    @StateObject private var vpn = VPNManager()
    @State private var confText = """
    [Interface]
    PrivateKey = REPLACE_CLIENT_PRIVATE_KEY
    Address = 10.8.0.2/24
    DNS = 1.1.1.1

    [Peer]
    PublicKey = REPLACE_SERVER_PUBLIC_KEY
    Endpoint = vpn.example.com:51820
    AllowedIPs = 0.0.0.0/0
    PersistentKeepalive = 25
    """
    @State private var statusText = "未连接"
    @State private var message = ""

    var body: some View {
        NavigationStack {
            Form {
                Section("状态") {
                    Text(statusText)
                        .foregroundStyle(vpn.isConnected ? .green : .secondary)
                    if !vpn.lastStats.isEmpty {
                        Text(vpn.lastStats)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }

                Section("WireGuard 配置") {
                    TextEditor(text: $confText)
                        .font(.system(.footnote, design: .monospaced))
                        .frame(minHeight: 180)
                    Button("保存到 App Group") {
                        do {
                            try vpn.saveConfig(confText)
                            message = "已保存到 App Group，扩展可读取。"
                        } catch {
                            message = "保存失败：\(error.localizedDescription)"
                        }
                    }
                }

                Section {
                    Button(vpn.isConnected ? "断开" : "连接") {
                        Task {
                            if vpn.isConnected {
                                await vpn.disconnect()
                                statusText = "未连接"
                            } else {
                                statusText = "正在连接…"
                                do {
                                    try vpn.saveConfig(confText)
                                    try await vpn.connect()
                                    statusText = "已连接"
                                    message = ""
                                } catch {
                                    statusText = "未连接"
                                    message = "错误：\(error.localizedDescription)"
                                }
                            }
                        }
                    }
                    .disabled(confText.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                }

                if !message.isEmpty {
                    Section("提示") {
                        Text(message)
                            .font(.footnote)
                            .foregroundStyle(.orange)
                    }
                }

                Section("说明") {
                    Text("本示例通过 Network Extension（Packet Tunnel）工作，不能用纯 Go/Fyne 在 iOS 上直接创建 TUN。请先按 INTEGRATION.md 完成签名、App Group 与 WGMobile.xcframework 链接。摄像头扫码可在后续版本加入；当前请粘贴完整配置文本。")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .navigationTitle("Cown VPN")
            .task {
                await vpn.refreshStatus()
                statusText = vpn.isConnected ? "已连接" : "未连接"
            }
        }
    }
}

#Preview {
    ContentView()
}
