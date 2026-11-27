# Cown VPN Android

Android Studio 工程。用 gomobile 把现有的 `pkg/wgmobile` 打成 `wgmobile.aar`，应用负责 VpnService（地址、DNS、路由、通知），Go 负责在 TUN fd 上跑 wireguard-go。

AAR **不提交**。没有手机或模拟器时，不需要安装或运行应用。

## 在 Mac 上生成 AAR

本机需要已经装好：

| 工具 | 作用 |
|------|------|
| Go 1.26+ | 编译 `pkg/wgmobile`（`go.mod` 的 go 行）。Go 1.27 可以 |
| Android SDK | `ANDROID_HOME`，含 API 35 |
| Android NDK | `ANDROID_NDK_HOME`（Android Studio → SDK Manager → NDK Side by side） |
| gomobile 与 gobind | 绑定 Go 为 AAR |

在仓库根目录：

```bash
export ANDROID_HOME="$HOME/Library/Android/sdk"
export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/<version>"
export PATH="$(go env GOPATH)/bin:$PATH"

go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
gomobile init

./android/scripts/build_aar.sh
```

脚本会检查 `go`、`gomobile`、`gobind`、`ANDROID_HOME`、`ANDROID_NDK_HOME`，缺任何一项都会直接退出并说明缺什么。`go.mod` 里已经有 `tool golang.org/x/mobile/cmd/gobind`，干净检出不需要再执行 `go get -tool`。成功后写出：

```text
android/app/libs/wgmobile.aar
```

`make android-aar` 是同一条命令。

## 用 Android Studio 打开

1. Android Studio → **Open** → 选择仓库里的 **`android/`** 目录（不是仓库根目录）。
2. 若提示安装 SDK 35，按提示安装。不要提交 `local.properties`。
3. 先跑过上面的脚本，否则 Gradle 的 `preBuild` 会因为缺少 AAR 失败。
4. Run **debug** 变体。`minSdk` 26，`targetSdk` / `compileSdk` 35。

## 应用在做什么

- **导入**：选择一个 `.conf` 文件，或把 WireGuard 配置贴进文本框。占位符不是真实密钥。
- **验证**：调用 `parseConf`（地址、DNS、AllowedIPs、Endpoint）和 `buildUAPI`（检查密钥能解析）。界面不展示 UAPI，里面有密钥材料。
- **连接**：先请求通知权限（Android 13+），再请求 VPN 权限。通过后前台服务建立 TUN，把 fd 交给 `startWithFD`。
- **断开**：停止 Go 隧道并关闭前台服务。系统设置里关掉 VPN 时，服务会收到 `onRevoke`。

配置保存在应用私有目录 `last.conf`，`allowBackup` 为 false。不要把真实 `.conf` 放进 git。

## gomobile 方法（Java / Kotlin）

包名 `wgmobile.Wgmobile`。Go 的 `int` 在 Java 侧是 `long`。

| Go | Kotlin |
|----|--------|
| `ParseConf` | `Wgmobile.parseConf(text): String` |
| `BuildUAPI` | `Wgmobile.buildUAPI(text): String` |
| `StartWithFD` | `Wgmobile.startWithFD(text, fd)` |
| `SetProtector` / `ClearProtector` | `setProtector` / `clearProtector` |
| `SocketV4` / `SocketV6` | `socketV4()` / `socketV6(): Long`，没有则为 `-1` |
| `Stop` / `Status` / `StatsJSON` | `stop` / `status` / `statsJSON` |

`Start` 会自己创建 TUN，在 Android 上不可用。连接必须走 `StartWithFD`。调用返回后可以关闭原来的 `ParcelFileDescriptor`；Go 已经 `dup` 了 fd。`Protector.protect` 对应 `VpnService.protect`，避免 UDP 握手被吸进隧道。

## 没有跑过的部分

云端环境没有 Android SDK、NDK、gomobile，也没有设备。AAR 和 APK 需要在 Mac 上按上面的步骤生成。不要把「已在真机连接」当作这一变更的完成条件。
