<div align="center">

# agy-swap (한국어)

### 터미널에서 Google Antigravity CLI와 IDE 계정을 함께 전환하고 할당량 확인하기

[English](../../README.md) · [ไทย](README.th.md) · [日本語](README.ja.md) · [简体中文](README.zh-CN.md) · **한국어**

</div>

**agy-swap**은 한 컴퓨터에서 여러 Google Antigravity 계정을 관리하는 오픈소스 터미널 UI 및 CLI입니다. Gemini, Claude/GPT, Gemini 4 Argon 풀의 5시간 및 주간 남은 할당량과 로컬 리셋 시각을 실시간으로 확인하고, 로그아웃이나 브라우저 재인증 없이 `agy` CLI와 (macOS 및 Linux의) Antigravity IDE를 한 번에 전환합니다.

- 한국어 웹사이트: https://agy-swap.aklkbqx.com/ko/
- 설치 안내: https://agy-swap.aklkbqx.com/ko/install.html
- 가이드 및 문제 해결 (영어): https://agy-swap.aklkbqx.com/guides/

## 주요 기능

| 다중 계정 사용 시 불편함 | agy-swap 해결 방식 |
| :--- | :--- |
| 할당량이 다 떨어질 때마다 로그아웃 후 브라우저에서 다시 로그인 | `agy-swap add`로 계정을 한 번만 저장하고 `Enter` -> `Switch`로 `agy` CLI와 Antigravity IDE (macOS 및 Linux)를 함께 전환 |
| 다른 계정에 Gemini, Claude/GPT 또는 Argon 할당량이 얼마나 남았는지 불명확 | 5시간·주간 게이지, 리셋 카운트다운 및 `READY`, `LOW`, `GEMINI ONLY`, `CLAUDE/GPT ONLY`, `LIMITED`, `VERIFY` 배지 표시 |
| 장시간 코딩이나 자동화 중 속도 제한 (HTTP 429) 발생 | `agy-swap next --family claude`로 자동 전환, `agy-swap limit`로 수동 대기 시간 지정, `profile` / `bind`로 프로젝트별 연결 |
| 다른 폴더에서 실행 중인 `agy`가 백그라운드 갱신 시 새 활성 계정을 덮어씀 | 세션 보호(`--sync-session`)가 갱신된 토큰을 일치하는 vault 항목으로 회수해 활성 계정을 보호 |
| 새 컴퓨터 설정 및 사내 TLS 검사 프록시 환경 지원 | `agy-swap install-agy`(SHA-512 검증)로 공식 CLI 설치, `statusline` 지원, AES-GCM 암호화 백업 및 `AGY_SWAP_CA_BUNDLE` 지원 |

## 설치 방법

macOS / Linux (Shell, SHA-256 검증):

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://agy-swap.aklkbqx.com/install.sh | bash
```

Homebrew (macOS / Linux):

```bash
brew install aklkbqx/agy-swap/agy-swap
```

Windows PowerShell (SHA-256 검증):

```powershell
irm https://agy-swap.aklkbqx.com/install.ps1 | iex
```

소스에서 빌드 (Go 1.27.4 이상 필요):

```bash
go install github.com/aklkbqx/agy-swap/v2/cmd/agy-swap@latest
```

## 처음 실행하기

1. `agy` CLI가 아직 없다면 `agy-swap install-agy`로 공식 바이너리를 설치합니다.
2. `agy-swap add`를 실행해 브라우저 로그인으로 각 Google 계정을 저장합니다.
3. `agy-swap`으로 TUI를 열고 여유가 있는 계정에서 `Enter` -> `Switch`를 선택합니다.

## 자주 쓰는 명령어

```bash
agy-swap                        # 대화형 터미널 UI 열기
agy-swap list                   # 저장된 계정 상태 및 할당량 목록 확인
agy-swap switch 2               # 이메일, 별칭 또는 번호로 계정 전환
agy-swap next --family claude   # 할당량 갱신 후 여유 있는 다음 계정으로 전환
agy-swap limits --refresh       # 5시간, 주간 및 Argon 할당량 세부 창 확인
agy-swap limit set 1 6h         # 특정 계정에 수동 대기 시간 설정 또는 해제
agy-swap verify dev@company.com # VERIFY 상태 계정의 Google 인증 페이지 열기 (TUI는 w 키)
agy-swap statusline install     # Starship, tmux, zsh 프롬프트에 상태줄 추가
agy-swap doctor                 # 로컬 파일, 인증 정보 및 프록시 연결 상태 점검
```

macOS 또는 Linux에서 두 개의 `agy` CLI 터미널을 서로 다른 Google 계정으로 동시에 실행하려면 별도의 `HOME` 디렉터리, `antigravity-keyring-unavailable` 파일, `AGY_SWAP_SKIP_SYNC=1 WSL_INTEROP=1` 설정을 사용합니다. 자세한 명령은 [README.md](../../README.md#run-two-agy-cli-sessions-on-different-accounts-at-once)에서 확인할 수 있습니다.

## 보안 및 토큰 저장

`agy-swap`은 로컬에서만 동작하며 텔레메트리를 전송하지 않습니다. 저장된 계정 토큰은 본인만 읽을 수 있는 `vault.json`(권한 `0600`, 암호화되지 않음)에 저장되며, macOS Keychain, Windows 자격 증명 관리자 또는 Linux Secret Service를 사용하려면 `AGY_SWAP_VAULT=keychain`을 설정하세요. 자세한 내용은 [SECURITY.md](../../SECURITY.md)에 정리되어 있습니다.
