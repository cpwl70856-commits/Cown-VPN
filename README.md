# Cown VPN

Cown VPN 是跨平台 WireGuard **桌面**客户端，基于 [wireguard-go](https://git.zx2c4.com/wireguard-go/) + [Fyne](https://fyne.io/)。

**Module:** `github.com/user/mywgclient`

| 平台 | 桌面支持 |
|------|----------|
| Linux | 完整（TUN + `ip route`） |
| macOS | 完整（`utun` + `route`/`ifconfig`） |
| Windows | 完整（需 **Wintun** + 管理员） |

> iOS/iPadOS 为独立 Network Extension 模板，见 `platforms/ios/`。  
> Android 客户端在 `android/`。AAR 不入库，在已安装 Go、gomobile、Android SDK 与 NDK 的 Mac 上执行 `./android/scripts/build_aar.sh`。详见 [android/README.md](android/README.md)。

---

## 普通用户：最快上手（约 1 分钟）

### 1. 安装 / 编译

```bash
go mod tidy
go run ./cmd/keygen          # 生成 PrivateKey / PublicKey
go build -o bin/mywgclient ./cmd/client
go build -o bin/mywgserver ./cmd/server
```

### 2. 提权运行（必须）

| 系统 | 做法 |
|------|------|
| Linux | `sudo ./bin/mywgclient`，或界面「复制提权命令」 |
| macOS | `sudo ./bin/mywgclient`，或「复制提权命令」 |
| Windows | 安装 [Wintun](https://www.wintun.net) 的 `wintun.dll` 到 exe 同目录，**以管理员身份运行** |

### 3. 配置（任选一种）

1. **快速向导**（Endpoint + 服务器公钥 + 本机私钥）
2. **剪贴板一键导入**（先复制完整 `.conf` / `vpn://` 文本）
3. **粘贴配置…** / **拖放 `.conf` 到窗口**
4. **导入二维码图片…** / **导入 .conf…**

可选：点 **测试配置**（只验证握手，**不**安装 `0.0.0.0/0` 路由）。

### 4. 连接

选 **分流** 或 **全局** → **连接**。可开「断线自动重连」。托盘可连接/断开；**退出会清理本会话路由**。

### 5. 联调测试服务器

```bash
# 终端 A
sudo ./bin/mywgserver -config config/server.example.conf

# 终端 B
sudo ./bin/mywgclient
# 或：sudo ./bin/mywgclient -config config/client.example.conf -route-mode split
```

请用 `keygen` 生成的真实密钥替换示例占位符。

---

## 桌面功能一览

- 首次引导 + 快速向导；剪贴板一键导入 / 粘贴 / 拖放 `.conf` / 二维码图片
- **多档案**：切换、重命名、删除、设默认、导出 `.conf`
- **连接诊断**：密钥、权限、Wintun、UDP、本机网卡、**路由状态** + 修复建议
- **测试配置**：无全局路由的握手验证；**测试 TUN** 查权限
- 分流 / 全局路由；断开、退出、信号、异常断开时**清理已跟踪路由**
- 流量统计 + **速率（B/s）**、最近握手、已连接时长
- 日志：级别过滤、清空、导出
- 系统托盘实时状态；开机自启（Linux XDG / macOS LaunchAgent / Windows Run，尽力而为）
- 跟随系统浅色/深色主题；无头：`-config` + `-route-mode`

---

## FAQ（桌面）

**Q: 连上后没网 / 断开后没网？**  
全局模式会装 `0.0.0.0/1` + `128.0.0.0/1`。正常断开/退出会清理；若进程被强杀，可用「全局路由命令」里的删除行手动清理，或重启网络。

**Q: 一直「尚无握手」？**  
多为密钥不配对、Endpoint 错误或服务器未放行 UDP。用「连接诊断」+「测试配置」。服务器对错误密钥通常**静默**。

**Q: Windows 提示 TUN / Wintun？**  
管理员运行；把对应架构 `wintun.dll` 放到 exe 旁（见 https://www.wintun.net），或安装官方 WireGuard。

**Q: 为什么 DNS 没变？**  
桌面端**不会**自动改系统 DNS / split DNS；如需请在系统网络设置或配置文件侧自行处理。

**Q: 开机自启连不上？**  
自启通常无管理员权限，创建 TUN 会失败。Linux/macOS 需额外包装 sudo/pkexec；Windows 建议登录后手动以管理员启动，或配置计划任务提权。

**Q: 端口占用？**  
Listen Port 留空由系统分配，或关掉占用该 UDP 端口的进程。

---

## 权限与路由

| 系统 | 要求 |
|------|------|
| Linux | root 或 `CAP_NET_ADMIN` |
| macOS | root（或相应 entitlement） |
| Windows | 管理员 + `wintun.dll` |

| 模式 | 行为 |
|------|------|
| **分流** | 仅为 AllowedIPs 中非默认网段加路由 |
| **全局** | UAPI `0.0.0.0/0`，OS 侧 `0.0.0.0/1` + `128.0.0.0/1`（便于安全拆除） |
| **测试配置** | 不安装 OS 路由，握手后拆除 |

配置目录：`~/.config/mywgclient/`（`settings.json`、`profiles/`、`profile.conf`）。

## Android

在装有 **Go、gomobile、Android Studio（SDK 35 + NDK）** 的 Mac 上，于仓库根目录：

```bash
export ANDROID_HOME="$HOME/Library/Android/sdk"
export ANDROID_NDK_HOME="$ANDROID_HOME/ndk/<version>"
export PATH="$(go env GOPATH)/bin:$PATH"
go install golang.org/x/mobile/cmd/gomobile@latest
go install golang.org/x/mobile/cmd/gobind@latest
gomobile init
./android/scripts/build_aar.sh
```

脚本写出 `android/app/libs/wgmobile.aar`。用 Android Studio 打开 **`android/`** 目录，Run 调试包。界面可粘贴或导入 `.conf`，验证后连接；首次连接会请求 VPN 与通知权限。不要提交真实配置。当前没有设备或模拟器时，不必安装运行。

## 开发

```bash
go mod tidy
go build ./cmd/client ./cmd/server
go test ./internal/...
make package-linux   # 需 fyne CLI + assets/Icon.png
```

```
cmd/client/          桌面入口（GUI + -config 无头）
cmd/server/          测试服务端
cmd/keygen/          密钥生成
internal/config/     conf / 档案 / settings
internal/tunnel/     wireguard-go + 平台路由 + 清理
internal/ui/         Fyne 界面
internal/platform/   权限 / Wintun / 自启动
internal/diag/       连接诊断
internal/errs/       中文友好错误
pkg/wgmobile/        gomobile API（iOS XCFramework / Android AAR）
android/             Android Studio 工程与 build_aar.sh
platforms/ios/       iOS Network Extension 模板
```

## 许可与依赖

- `golang.zx2c4.com/wireguard` · `fyne.io/fyne/v2` · `github.com/makiuchi-d/gozxing`
- 示例客户端，请仅在有权访问的系统与密钥上使用。
