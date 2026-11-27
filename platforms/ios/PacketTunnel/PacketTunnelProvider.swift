import Foundation
import NetworkExtension
import os.log

#if canImport(WGMobile)
import WGMobile
#endif

/// Packet Tunnel provider for mywgclient on iOS / iPadOS.
///
/// Wire this class as the Network Extension principal class
/// (`NSExtensionPrincipalClass` = `$(PRODUCT_MODULE_NAME).PacketTunnelProvider`).
///
/// Flow:
/// 1. Read WireGuard conf from the App Group container
/// 2. Apply `NEPacketTunnelNetworkSettings` (address / DNS / routes)
/// 3. Start the Go core (`WGMobile`) or WireGuardKit and bridge `packetFlow`
class PacketTunnelProvider: NEPacketTunnelProvider {
    private let log = OSLog(subsystem: "com.user.mywgclient", category: "tunnel")
    private let appGroupID = "group.com.user.mywgclient"
    private let confFileName = "wg0.conf"

    private var confText: String = ""

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        os_log("startTunnel", log: log, type: .info)

        do {
            confText = try loadConfigFromAppGroup()
        } catch {
            os_log("load config failed: %{public}@", log: log, type: .error, String(describing: error))
            completionHandler(error)
            return
        }

        applyNetworkSettings(from: confText) { [weak self] error in
            guard let self else { return }
            if let error {
                completionHandler(error)
                return
            }
            do {
                try self.startEngine(confText: self.confText)
                self.readPackets()
                completionHandler(nil)
            } catch {
                os_log("engine start failed: %{public}@", log: log, type: .error, String(describing: error))
                completionHandler(error)
            }
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        os_log("stopTunnel reason=%{public}d", log: log, type: .info, reason.rawValue)
        stopEngine()
        completionHandler()
    }

    override func handleAppMessage(_ messageData: Data, completionHandler: ((Data?) -> Void)?) {
        // Host app may request status / stats JSON.
        let reply: Data?
        #if canImport(WGMobile)
        let json = WgmobileStatsJSON()
        reply = json.data(using: .utf8)
        #else
        reply = #"{"state":"unknown","note":"link WGMobile.xcframework"}"#.data(using: .utf8)
        #endif
        completionHandler?(reply)
    }

    // MARK: - App Group config

    private func loadConfigFromAppGroup() throws -> String {
        guard let container = FileManager.default.containerURL(forSecurityApplicationGroupIdentifier: appGroupID) else {
            throw makeError("无法访问 App Group（请检查 entitlements 与 group id）")
        }
        let url = container.appendingPathComponent(confFileName)
        if FileManager.default.fileExists(atPath: url.path) {
            return try String(contentsOf: url, encoding: .utf8)
        }
        // Fallback: UserDefaults suite
        if let defaults = UserDefaults(suiteName: appGroupID),
           let text = defaults.string(forKey: "wireguard_conf"),
           !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            return text
        }
        throw makeError("App Group 中未找到配置。请先在主 App 导入并保存 WireGuard 配置。")
    }

    // MARK: - Network settings

    private func applyNetworkSettings(from conf: String, completion: @escaping (Error?) -> Void) {
        var remote = "127.0.0.1"
        var address = "10.8.0.2"
        var mtu: NSNumber = 1420
        var dns: [String] = []
        var included: [NEIPv4Route] = [NEIPv4Route.default()]

        #if canImport(WGMobile)
        if let hintsJSON = try? WgmobileParseConf(conf),
           let data = hintsJSON.data(using: .utf8),
           let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any] {
            if let ep = obj["endpoint"] as? String, !ep.isEmpty {
                remote = ep.split(separator: ":").first.map(String.init) ?? remote
            }
            if let ip = obj["address_ip"] as? String, !ip.isEmpty {
                address = ip
            }
            if let m = obj["mtu"] as? Int, m > 0 {
                mtu = NSNumber(value: m)
            }
            if let d = obj["dns"] as? [String] {
                dns = d
            }
            if let allowed = obj["allowed_ips"] as? [String], !allowed.isEmpty {
                included = allowed.compactMap { cidr in
                    let parts = cidr.split(separator: "/")
                    guard parts.count == 2, let prefix = Int(parts[1]) else { return nil }
                    let dest = String(parts[0])
                    let mask = Self.ipv4Mask(prefixLength: prefix)
                    if dest == "0.0.0.0" {
                        return NEIPv4Route.default()
                    }
                    return NEIPv4Route(destinationAddress: dest, subnetMask: mask)
                }
                if included.isEmpty {
                    included = [NEIPv4Route.default()]
                }
            }
        }
        #else
        // Naive parse for skeleton builds without the Go framework.
        if let addrLine = conf.split(separator: "\n").first(where: { $0.lowercased().contains("address") }) {
            let v = addrLine.split(separator: "=").last?.trimmingCharacters(in: .whitespaces) ?? ""
            if let ip = v.split(separator: "/").first, !ip.isEmpty {
                address = String(ip)
            }
        }
        if let epLine = conf.split(separator: "\n").first(where: { $0.lowercased().contains("endpoint") }) {
            let v = epLine.split(separator: "=").last?.trimmingCharacters(in: .whitespaces) ?? ""
            if let host = v.split(separator: ":").first, !host.isEmpty {
                remote = String(host)
            }
        }
        #endif

        let settings = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: remote)
        settings.mtu = mtu

        let ipv4 = NEIPv4Settings(addresses: [address], subnetMasks: ["255.255.255.0"])
        ipv4.includedRoutes = included
        settings.ipv4Settings = ipv4

        if !dns.isEmpty {
            let dnsSettings = NEDNSSettings(servers: dns)
            settings.dnsSettings = dnsSettings
        }

        setTunnelNetworkSettings(settings, completionHandler: completion)
    }

    private static func ipv4Mask(prefixLength: Int) -> String {
        var mask: UInt32 = 0
        if prefixLength > 0 {
            mask = UInt32.max << (32 - min(prefixLength, 32))
        }
        return String(format: "%d.%d.%d.%d",
                      (mask >> 24) & 0xff,
                      (mask >> 16) & 0xff,
                      (mask >> 8) & 0xff,
                      mask & 0xff)
    }

    // MARK: - Go / engine

    private func startEngine(confText: String) throws {
        #if canImport(WGMobile)
        // 在 NE 内 CreateTUN 通常不可用；生产请用 BuildUAPI + WireGuardKit / 自定义 packet 桥接。
        // Start 便于早期联调；即使 Start 受环境限制，ParseConf / BuildUAPI 仍可用于填设置。
        _ = try WgmobileBuildUAPI(confText)
        do {
            try WgmobileStart(confText)
            os_log("WGMobile Start 成功", log: log, type: .info)
        } catch {
            os_log("WGMobile Start 跳过（NE 环境常见）：%{public}@", log: log, type: .info, String(describing: error))
        }
        #else
        os_log("未链接 WGMobile — 仅应用了 Network Settings。详见 INTEGRATION.md", log: log, type: .info)
        #endif
    }

    private func stopEngine() {
        #if canImport(WGMobile)
        WgmobileStop()
        #endif
    }

    // MARK: - Packet loop (bridge stub)

    private func readPackets() {
        packetFlow.readPackets { [weak self] packets, protocols in
            guard let self else { return }
            // Production: feed IP packets into wireguard-go / WireGuardKit,
            // send ciphertext via UDP, write decrypted packets back:
            //   self.packetFlow.writePackets(decrypted, withProtocols: protocols)
            _ = packets
            _ = protocols
            self.readPackets()
        }
    }

    private func makeError(_ message: String) -> NSError {
        NSError(domain: "com.user.mywgclient.PacketTunnel", code: 1,
                userInfo: [NSLocalizedDescriptionKey: message])
    }
}
