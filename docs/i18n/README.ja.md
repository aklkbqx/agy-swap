<div align="center">

# agy-swap (日本語)

### ターミナルから Google Antigravity の CLI と IDE アカウントを切り替え

[English](../../README.md) · [ไทย](README.th.md) · **日本語** · [简体中文](README.zh-CN.md) · [한국어](README.ko.md)

</div>

**agy-swap** は 1 台のマシンで複数の Google Antigravity アカウントを管理するオープンソースの TUI / CLI です。Gemini・Claude/GPT・Gemini 4 Argon の 5 時間枠と週間クォータおよびローカルリセット時刻を表示し、サインアウトやブラウザー操作なしで `agy` CLI と（macOS と Linux の）Antigravity IDE を切り替えます。

- 日本語サイト: https://agy-swap.aklkbqx.com/ja/
- インストール手順: https://agy-swap.aklkbqx.com/ja/install.html
- ガイド・トラブルシューティング (英語): https://agy-swap.aklkbqx.com/guides/

## 主な機能

| 複数アカウント利用時の課題 | agy-swap による解決 |
| :--- | :--- |
| クォータ切れのたびにブラウザーでサインインし直す手間 | `agy-swap add` で各アカウントを一度保存し、`Enter` -> `Switch` で `agy` CLI と Antigravity IDE (macOS / Linux) をまとめて切り替え |
| 他アカウントの Gemini・Claude/GPT・Argon 残量が分からない | 5 時間枠・週間枠・Argon 残量と `READY` / `LOW` / `GEMINI ONLY` / `CLAUDE/GPT ONLY` / `LIMITED` / `VERIFY` バッジを表示 |
| 自動実行や長時間作業中にレート制限 (HTTP 429) に達する | `agy-swap next --family claude` で自動ローテーション、`agy-swap limit` で手動クールダウン設定、`profile` / `bind` でプロジェクト別割り当て |
| 別フォルダーで起動中の `agy` が切り替え後のアカウントを上書きする | ランチャー保護 (`--sync-session`) がバックグラウンド更新トークンを一致する vault エントリーに回収し、アクティブセッションを維持 |
| 新規マシンの初期構築や企業 TLS プロキシ環境での導入 | `agy-swap install-agy` (SHA-512 検証) で公式 CLI を導入し、`statusline` 表示・AES-GCM 暗号化バックアップ・`AGY_SWAP_CA_BUNDLE` に対応 |

## インストール

macOS / Linux (シェル、SHA-256 検証付き):

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://agy-swap.aklkbqx.com/install.sh | bash
```

Homebrew (macOS / Linux):

```bash
brew install aklkbqx/agy-swap/agy-swap
```

Windows PowerShell (SHA-256 検証付き):

```powershell
irm https://agy-swap.aklkbqx.com/install.ps1 | iex
```

ソースからビルド (Go 1.27.4 以上が必要):

```bash
go install github.com/aklkbqx/agy-swap/v2/cmd/agy-swap@latest
```

## 初回セットアップ

1. `agy` が未導入の場合は `agy-swap install-agy` を実行します。
2. `agy-swap add` を実行し、ブラウザーで各 Google アカウントを保存します。
3. `agy-swap` で TUI を開き、残量のあるアカウントで `Enter` -> `Switch` を選択します。

## よく使うコマンド

```bash
agy-swap                        # 対話型 TUI を開く
agy-swap list                   # 保存済みアカウントとクォータ一覧
agy-swap switch 2               # メール・別名・番号で切り替え
agy-swap next --family claude   # クォータを更新して次の利用可能アカウントへ切り替え
agy-swap limits --refresh       # 5 時間枠・週間枠・Argon とリザーブ設定を表示
agy-swap limit set 1 6h         # 手動クールダウンの設定・解除
agy-swap verify dev@company.com # VERIFY 状態のアカウントの Google 認証ページを開く (TUI は w)
agy-swap statusline install     # Starship / tmux / zsh 用ステータスラインを設定
agy-swap doctor                 # ローカル設定・認証状態・ネットワーク接続を診断
```

macOS や Linux で 2 つの `agy` CLI セッションを別アカウントで同時起動する場合は、独立した `HOME`、`antigravity-keyring-unavailable` マーカー、および `AGY_SWAP_SKIP_SYNC=1 WSL_INTEROP=1` を使用します。詳細は [README.md](../../README.md#run-two-agy-cli-sessions-on-different-accounts-at-once) を参照してください。

## セキュリティとトークン保存

`agy-swap` は完全にローカルで動作し、利用テレメトリやクラッシュレポートを送信しません。対話画面やネットワーク系コマンド実行時のみ、キャッシュ付きの 24 時間ごとの最新バージョン確認 (`GET https://agy-swap.aklkbqx.com/version.json`、送信するのはバージョン・OS・CPU アーキテクチャ・導入経路のみでアカウントや端末 ID は含みません) を行います。この確認は `AGY_SWAP_NO_UPDATE_CHECK=1` または `DO_NOT_TRACK=1` で無効化できます。保存したアカウントトークンは本人のみ読み取り可能な `vault.json`（パーミッション `0600`、暗号化なし）に保管されます。macOS Keychain・Windows 資格情報マネージャー・Linux Secret Service を使う場合は `AGY_SWAP_VAULT=keychain` を設定してください。詳細は [SECURITY.md](../../SECURITY.md) を参照してください。
