Output "docs/assets/switch.mp4"

Require agy-swap

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 18
Set Width 1600
Set Height 960
Set Framerate 25
Set Padding 24
Set LineHeight 1.16
Set CursorBlink false
Set TypingSpeed 35ms
Set Theme { "name": "OperatorDeck", "black": "#1c2128", "red": "#f87171", "green": "#4ade80", "yellow": "#fbbf24", "blue": "#38bdf8", "magenta": "#c084fc", "cyan": "#66d9c4", "white": "#f0f6fc", "brightBlack": "#545d68", "brightRed": "#fca5a5", "brightGreen": "#86efac", "brightYellow": "#fde047", "brightBlue": "#8bb8ff", "brightMagenta": "#d8b4fe", "brightCyan": "#99f6e4", "brightWhite": "#ffffff", "background": "#0a0c10", "foreground": "#f0f6fc", "selectionBackground": "#2d333b", "cursor": "#ff891a" }

Hide
Type "clear && agy-swap" Enter
Sleep 1.2s
Show

# Overview Alpha User (Pro + Gemini 4 Argon panel in Wide mode)
Sleep 1.8s

# Navigate down to Beta User (GEMINI ONLY, Claude/GPT used up)
Down
Sleep 1.4s

# Navigate down to Gamma User (LOW, Argon 0%)
Down
Sleep 1.2s

# Move back up to Beta User
Up
Sleep 900ms

# Press Enter to open Switch Confirmation dialog
Enter
Sleep 1.3s

# Confirm Switch
Enter

# View toast notification & updated ACTIVE badge
Sleep 2.4s

# Clean exit
Type "q"
Sleep 300ms
