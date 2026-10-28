Output "docs/assets/switch.gif"
Output "docs/assets/switch.mp4"

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

# Overview Alpha User
Sleep 1.5s

# Navigate down to Beta User
Down
Sleep 1.2s

# Navigate down to Gamma User
Down
Sleep 1.2s

# Move back up to Beta User
Up
Sleep 1.0s

# Press Enter to open Switch Confirmation dialog
Enter
Sleep 1.2s

# Confirm Switch
Enter

# View toast notification & updated status badge
Sleep 2.5s

# Clean exit
Type "q"
Sleep 500ms
