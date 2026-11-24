<div align="center">

# agy-swap (简体中文)

### 在终端同步切换 Google Antigravity CLI 与 IDE 多账号并实时监控配额

[English](../../README.md) · [ไทย](README.th.md) · [日本語](README.ja.md) · **简体中文** · [한국어](README.ko.md)

</div>

**agy-swap** 用于在一台设备上管理多个 Google Antigravity 账号，实时查看 Gemini、Claude/GPT 与 Gemini 4 Argon 配额池的 5 小时及每周剩余容量和本地重置时间，并在不退出登录、不打开浏览器的情况下同步切换 `agy` CLI 与（macOS 和 Linux 上的）Antigravity IDE。

- 中文主页：https://agy-swap.aklkbqx.com/zh/
- 安装指南：https://agy-swap.aklkbqx.com/zh/install.html
- 专题指南与故障排查 (英文)：https://agy-swap.aklkbqx.com/guides/

## 核心能力

| 多账号使用痛点 | agy-swap 如何解决 |
| :--- | :--- |
| 配额耗尽时需要反复退出并在浏览器重新登录 | 运行 `agy-swap add` 保存一次各账号，之后按 `Enter` -> `Switch` 同步切换 `agy` CLI 与 Antigravity IDE (macOS 和 Linux) |
| 不清楚哪个备用账号的 Gemini、Claude/GPT 或 Argon 还有余量 | 实时显示 5 小时与每周配额池进度条、重置时间以及 `READY` / `LOW` / `GEMINI ONLY` / `CLAUDE/GPT ONLY` / `LIMITED` / `VERIFY` 标签 |
| 长时间编码或脚本运行时中途遇到限流 (HTTP 429) | 使用 `agy-swap next --family claude` 自动轮换，使用 `agy-swap limit` 设置冷却期，并用 `profile` / `bind` 按目录绑定策略 |
| 其他目录打开的 `agy` 在后台刷新令牌时覆盖当前账号 | 会话守卫 (`--sync-session`) 按 `refresh_token` 回收后台刷新令牌回对应的 vault 记录，不覆盖当前活跃会话 |
| 新机器快速部署或在企业 TLS 检查代理网络下使用 | 通过 `agy-swap install-agy` (校验 SHA-512) 安装官方 CLI，支持 `statusline`、AES-GCM 加密备份与 `AGY_SWAP_CA_BUNDLE` |

## 安装方式

macOS / Linux (Shell，校验 SHA-256):

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://agy-swap.aklkbqx.com/install.sh | bash
```

Homebrew (macOS / Linux):

```bash
brew install aklkbqx/agy-swap/agy-swap
```

Windows PowerShell (校验 SHA-256):

```powershell
irm https://agy-swap.aklkbqx.com/install.ps1 | iex
```

从源码安装 (需要 Go 1.27.4 或更高版本):

```bash
go install github.com/aklkbqx/agy-swap/v2/cmd/agy-swap@latest
```

## 快速上手

1. 若尚未安装 `agy`，运行 `agy-swap install-agy` 安装官方 Google Antigravity CLI。
2. 运行 `agy-swap add` 并通过浏览器保存每个 Google 账号。
3. 运行 `agy-swap` 打开终端界面，选中仍有额度的账号，按 `Enter` 后选择 `Switch`。

## 常用命令

```bash
agy-swap                        # 打开交互式终端界面 (TUI)
agy-swap list                   # 查看已保存账号的状态和配额
agy-swap switch 2               # 按邮箱、别名或序号切换账号
agy-swap next --family claude   # 刷新配额并轮换到下一个可用账号
agy-swap limits --refresh       # 查看详细的 5 小时、每周与 Argon 配额窗口
agy-swap limit set 1 6h         # 设置或清除手动冷却时间
agy-swap verify dev@company.com # 打开 Google 账号验证页面 (TUI 中按 w)
agy-swap statusline install     # 为 Starship、tmux 或 zsh 配置状态栏
agy-swap doctor                 # 检查本地存储、凭证状态与网络连通性
```

如果需要在 macOS 或 Linux 上同时运行两个使用不同 Google 账号的 `agy` CLI 终端，请为第二个终端设置独立的 `HOME`、`antigravity-keyring-unavailable` 标记以及 `AGY_SWAP_SKIP_SYNC=1 WSL_INTEROP=1`，完整步骤见 [README.md](../../README.md#run-two-agy-cli-sessions-on-different-accounts-at-once)。

## 安全与凭证存储

`agy-swap` 完全在本地运行，不发送任何使用遥测或崩溃报告。仅在交互界面及联网命令期间每 24 小时执行一次带缓存的新版本检查 (`GET https://agy-swap.aklkbqx.com/version.json`，仅发送版本号、系统、架构和安装渠道，不包含账号或设备标识)，可通过设置 `AGY_SWAP_NO_UPDATE_CHECK=1` 或 `DO_NOT_TRACK=1` 关闭。已保存的账号令牌默认存储在仅当前用户可读的 `vault.json` 文件（权限 `0600`，未加密）中；设置 `AGY_SWAP_VAULT=keychain` 可改为保存到 macOS Keychain、Windows 凭据管理器或 Linux Secret Service。详细说明见 [SECURITY.md](../../SECURITY.md)。
