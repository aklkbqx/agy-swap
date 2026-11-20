<div align="center">

# agy-swap (ภาษาไทย)

### สลับบัญชี Google Antigravity ทั้ง CLI และ IDE จากเทอร์มินัลเดียว

[English](../../README.md) · **ไทย** · [日本語](README.ja.md) · [简体中文](README.zh-CN.md) · [한국어](README.ko.md)

</div>

**agy-swap** จัดการบัญชี Google Antigravity หลายบัญชีในเครื่องเดียว แสดงโควตาคงเหลือรอบ 5 ชั่วโมงและรายสัปดาห์ของพูล Gemini, Claude/GPT และ Gemini 4 Argon พร้อมเวลารีเซ็ตตามเวลาเครื่อง และสลับทั้ง `agy` CLI และ Antigravity IDE (บน macOS และ Linux) จากเทอร์มินัลเดียวโดยไม่ต้องออกจากระบบหรือเปิดเบราว์เซอร์

- เว็บไซต์ภาษาไทย: https://agy-swap.aklkbqx.com/th/
- คู่มือติดตั้ง: https://agy-swap.aklkbqx.com/th/install.html
- คู่มือเจาะลึกและการแก้ปัญหา: https://agy-swap.aklkbqx.com/guides/

## จุดเด่นหลัก

| ปัญหาเมื่อใช้หลายบัญชี | วิธีแก้ด้วย agy-swap |
| :--- | :--- |
| ต้องออกจากระบบและล็อกอินใหม่ในเบราว์เซอร์เมื่อโควตาหมด | บันทึกแต่ละบัญชีด้วย `agy-swap add` แล้วกด `Enter` -> `Switch` เพื่อสลับทั้ง `agy` CLI และ Antigravity IDE (macOS และ Linux) |
| ไม่รู้ว่าบัญชีไหนเหลือโควตา Gemini, Claude/GPT หรือ Argon เท่าไร | แสดงเกจรอบ 5 ชั่วโมงและรายสัปดาห์พร้อมป้าย `READY`, `LOW`, `GEMINI ONLY`, `CLAUDE/GPT ONLY`, `LIMITED` และ `VERIFY` |
| ติดลิมิตกลางงานระหว่างรันสคริปต์ | หมุนบัญชีอัตโนมัติด้วย `agy-swap next --family claude`, ตั้งพักชั่วคราวด้วย `agy-swap limit set` และผูกโฟลเดอร์ด้วย `profile` / `bind` |
| เปิด `agy` ค้างไว้หลายโฟลเดอร์แล้วระบบแอบรีเฟรชโทเคนทับบัญชีที่สลับ | Session guard (`--sync-session`) เก็บโทเคนที่รีเฟรชกลับเข้า vault ตามบัญชีเดิมโดยไม่เปลี่ยนบัญชีหลักที่ใช้งานอยู่ |
| ตั้งค่า `agy` บนเครื่องใหม่หรือหลังพร็อกซีองค์กร | ติดตั้งตัวทางการด้วย `agy-swap install-agy` (ตรวจ SHA-512), ตั้งพรอมต์ด้วย `statusline`, สำรองเข้ารหัส AES-GCM และรองรับ `AGY_SWAP_CA_BUNDLE` |

## การติดตั้ง

บน macOS และ Linux (ตรวจเช็กซัม SHA-256):

```bash
curl -fsSL --proto '=https' --tlsv1.2 https://agy-swap.aklkbqx.com/install.sh | bash
```

ผ่าน Homebrew (macOS และ Linux):

```bash
brew install aklkbqx/agy-swap/agy-swap
```

บน Windows PowerShell (ตรวจเช็กซัม SHA-256):

```powershell
irm https://agy-swap.aklkbqx.com/install.ps1 | iex
```

ติดตั้งจากซอร์สโค้ด (ต้องใช้ Go 1.27.4 ขึ้นไป):

```bash
go install github.com/aklkbqx/agy-swap/v2/cmd/agy-swap@latest
```

## เริ่มต้นใช้งาน

1. ติดตั้ง Google Antigravity CLI (`agy`) หากยังไม่มีในเครื่องด้วย `agy-swap install-agy`
2. เข้าสู่ระบบและบันทึกบัญชีด้วย `agy-swap add`
3. เปิดหน้าจอ TUI ด้วย `agy-swap` เลือกบัญชีที่ยังมีโควตา กด `Enter` แล้วเลือก `Switch`

## คำสั่งที่ใช้บ่อย

```bash
agy-swap                        # เปิดหน้าจอเทอร์มินัล (TUI)
agy-swap list                   # แสดงรายการบัญชี สถานะ และแถบโควตา
agy-swap switch 2               # สลับบัญชีด้วยอีเมล ชื่อเล่น หรือเลขลำดับ
agy-swap next --family claude   # รีเฟรชและสลับไปบัญชีถัดไปที่ยังมีโควตา Claude/GPT
agy-swap limits --refresh       # ดูหน้าต่างโควตา 5 ชั่วโมง รายสัปดาห์ และ Argon แบบละเอียด
agy-swap limit set 1 6h         # ตั้งหรือล้างเวลาพักบัญชีด้วยตนเอง
agy-swap verify dev@company.com # เปิดหน้ายืนยันของ Google เมื่อบัญชีขึ้นสถานะ VERIFY (หรือกด w ใน TUI)
agy-swap statusline install     # แสดงโควตาบนพรอมต์ Starship, tmux หรือ zsh
agy-swap doctor                 # ตรวจไฟล์ระบบ ข้อมูลเข้าสู่ระบบ และพร็อกซี
```

หากต้องการรัน `agy` CLI สองหน้าต่างคนละบัญชีพร้อมกันบน macOS หรือ Linux ให้แยกโฟลเดอร์ `HOME` พร้อมไฟล์ `antigravity-keyring-unavailable` และกำหนด `AGY_SWAP_SKIP_SYNC=1 WSL_INTEROP=1` ตามรายละเอียดใน [README.md](../../README.md#run-two-agy-cli-sessions-on-different-accounts-at-once)

## ความปลอดภัยและการเก็บโทเคน

`agy-swap` ทำงานในเครื่องทั้งหมดและไม่มีการส่ง telemetry โทเคนที่บันทึกไว้เก็บอยู่ในไฟล์ `vault.json` (สิทธิ์ `0600` และไม่ได้เข้ารหัส) ซึ่งอ่านได้เฉพาะผู้ใช้ของคุณ หรือกำหนด `AGY_SWAP_VAULT=keychain` เพื่อเก็บใน macOS Keychain, Windows Credential Manager หรือ Linux Secret Service แทน ดูรายละเอียดทั้งหมดใน [SECURITY.md](../../SECURITY.md)
