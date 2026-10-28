Output "docs/assets/cli.gif"
Output "docs/assets/cli.mp4"

Require agy-swap

Set Shell "bash"
Set FontSize 15
Set Width 1200
Set Height 760
Set Padding 16
Set Theme { "background": "#0a0c10", "foreground": "#adbac7" }

Hide
Type "clear" Enter
Sleep 500ms
Show

# 1. View all accounts & quotas
Type "agy-swap list" Enter
Sleep 3.0s

# 2. Check current active account status
Type "agy-swap status" Enter
Sleep 2.5s

# 3. Rank accounts for the current policy
Type "agy-swap recommend" Enter
Sleep 3.0s

# 4. Switch explicitly
Type "agy-swap switch beta@example.invalid" Enter
Sleep 2.5s
