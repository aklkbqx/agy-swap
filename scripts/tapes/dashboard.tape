Output "docs/assets/dashboard.mp4"

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

# Inspect Wide Operator Deck
Sleep 1.2s

# Live account filter with '/'
Type "/"
Type "beta"
Sleep 1.0s
Escape
Sleep 600ms

# Manual quota refresh sweep with 'r'
Type "r"
Sleep 1.2s

# Open Command Palette with Ctrl+K and filter actions
Ctrl+K
Sleep 1.0s
Type "quota"
Sleep 1.3s
Backspace 5
Sleep 250ms
Type "profile"
Sleep 1.2s
Escape
Sleep 600ms

# Open Keyboard Guide with '?' (full width, no truncation)
Type "?"
Sleep 2.2s
Escape
Sleep 400ms

# Clean exit
Type "q"
Sleep 300ms
