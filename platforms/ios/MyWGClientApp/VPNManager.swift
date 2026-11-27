import Foundation
import NetworkExtension

/// 主 App 侧 NEVPNManager 封装：保存配置到 App Group，启停 Packet Tunnel。
@MainActor
final class VPNManager: ObservableObject {
    @Published var isConnected = false
    @Published var lastStats = ""

    private let appGroupID = "group.com.user.mywgclient"
    private let confFileName = "wg0.conf"
    /// 必须与 Xcode 中 Packet Tunnel 扩展的 Bundle Identifier 一致。
    private let tunnelBundleID = "com.user.mywgclient.PacketTunnel"

    private var manager: NEVPNManager { NEVPNManager.shared() }

    func saveConfig(_ text: String) throws {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !trimmed.isEmpty else {
            throw VPNError.emptyConfig
        }
        guard let container = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupID) else {
            throw VPNError.appGroupUnavailable
        }
        let url = container.appendingPathComponent(confFileName)
        try trimmed.write(to: url, atomically: true, encoding: .utf8)
        UserDefaults(suiteName: appGroupID)?.set(trimmed, forKey: "wireguard_conf")
    }

    func connect() async throws {
        try await load()

        let proto = NETunnelProviderProtocol()
        proto.providerBundleIdentifier = tunnelBundleID
        // serverAddress 仅用于系统 VPN 列表展示；真实 Endpoint 在 conf 内。
        proto.serverAddress = "mywgclient"
        proto.providerConfiguration = ["profile": "wg0"]

        manager.protocolConfiguration = proto
        manager.localizedDescription = "Cown VPN"
        manager.isEnabled = true

        try await savePreferences()
        try await load()
        try manager.connection.startVPNTunnel()
        isConnected = true
    }

    func disconnect() async {
        manager.connection.stopVPNTunnel()
        isConnected = false
        lastStats = ""
    }

    func refreshStatus() async {
        try? await load()
        switch manager.connection.status {
        case .connected, .connecting, .reasserting:
            isConnected = manager.connection.status == .connected
        default:
            isConnected = false
        }
        await fetchStatsFromExtension()
    }

    /// 通过 `handleAppMessage` 向扩展请求 WGMobile StatsJSON。
    func fetchStatsFromExtension() async {
        guard let conn = manager.connection as? NETunnelProviderSession else { return }
        do {
            let data: Data? = try await withCheckedThrowingContinuation { cont in
                do {
                    try conn.sendProviderMessage(Data("stats".utf8)) { reply in
                        cont.resume(returning: reply)
                    }
                } catch {
                    cont.resume(throwing: error)
                }
            }
            if let data, let text = String(data: data, encoding: .utf8) {
                lastStats = text
            }
        } catch {
            // 扩展未就绪时忽略。
        }
    }

    private func load() async throws {
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            manager.loadFromPreferences { error in
                if let error { cont.resume(throwing: error) }
                else { cont.resume() }
            }
        }
    }

    private func savePreferences() async throws {
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            manager.saveToPreferences { error in
                if let error { cont.resume(throwing: error) }
                else { cont.resume() }
            }
        }
    }
}

enum VPNError: LocalizedError {
    case emptyConfig
    case appGroupUnavailable

    var errorDescription: String? {
        switch self {
        case .emptyConfig: return "配置内容为空"
        case .appGroupUnavailable: return "无法访问 App Group，请检查 Capabilities / 签名"
        }
    }
}
