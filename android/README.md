# Cown VPN Android

Android Studio 工程。隧道使用 Maven 上的官方库 `com.wireguard.android:tunnel:1.0.20260102`（`GoBackend.setState`）。应用负责界面、`VpnService.prepare`、前台通知，以及把 wg-quick 文本交给这个库。

不需要本地 `wgmobile.aar`，也不需要运行 `./android/scripts/build_aar.sh`。

## 构建

需要 Android Studio（或命令行 Android SDK）和 JDK 17。`compileSdk` / `targetSdk` 35，`minSdk` 26。

1. 安装 Android SDK 35。不要提交 `local.properties`。
2. 用 Android Studio **Open** 仓库里的 **`android/`** 目录（不是仓库根目录）。
3. Run **debug**，或在 `android/` 下执行：

```bash
./gradlew assembleDebug testDebugUnitTest
```

Gradle 会从 Maven Central 下载 `com.wireguard.android:tunnel:1.0.20260102`。该库使用 Java 8+ API，工程已打开 core library desugaring（`desugar_jdk_libs:2.0.4`）。

## 应用在做什么

- **导入**：选择一个 `.conf` 文件，或把 WireGuard 配置贴进文本框。占位符不是真实密钥。
- **验证**：用 `com.wireguard.config.Config.parse` 检查密钥、Address、DNS、AllowedIPs、Endpoint。缺配置或解析失败时，界面显示中文错误。
- **连接**：先请求通知权限（Android 13+），再 `VpnService.prepare`。用户同意后，后台线程调用 `GoBackend.setState(tunnel, UP, config)`。库会启动 `CownVpnService`（`GoBackend.VpnService` 的子类）并建立 TUN。
- **通知**：VPN 运行期间前台服务保持一条常驻通知。Android 14+ 使用 `systemExempted`（已授权的 VPN 应用）。
- **断开**：`GoBackend.setState(tunnel, DOWN, null)`，并停掉前台服务。系统设置里关掉 VPN 时，服务收到 `onRevoke`，界面显示「系统已关闭 VPN」。

配置保存在应用私有目录 `last.conf`，`allowBackup` 为 false。不要把真实 `.conf` 放进 git。

## `build_aar.sh` 不再被应用使用

`android/scripts/build_aar.sh` 和 `make android-aar` 仍可以把 `pkg/wgmobile` 打成 AAR，那是 gomobile 绑定，给仍想要这份绑定的人用。Android 应用的 Gradle 工程不读取 `android/app/libs/wgmobile.aar`。没有 Go、gomobile 或 NDK 也可以编译安装这个应用。

iOS 仍使用 `pkg/wgmobile`，见 `platforms/ios/`。
