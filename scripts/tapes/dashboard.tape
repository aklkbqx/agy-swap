Output "docs/assets/dashboard.gif"
Output "docs/assets/dashboard.mp4"

Require agy-swap

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 760
Set Padding 16
Set Theme { "background": "#0a0c10", "foreground": "#adbac7" }

Hide
Type "clear && agy-swap" Enter
Sleep 1.5s
Show

# Inspect Accounts and Quota Deck
Sleep 2.0s
Down
Sleep 1.2s

# Open Help Overlay with '?'
Type "?"
Sleep 2.5s
Escape
Sleep 1.0s

# Open Command Palette with Ctrl+K
Ctrl+K
Sleep 1.5s
Type "quota"
Sleep 2.0s
Escape
Sleep 1.2s

# Clean exit
Type "q"
Sleep 500ms
