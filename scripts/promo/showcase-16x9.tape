Output "dist/media/showcase-16x9.mp4"

Require agy-swap

Set Shell "bash"
Set FontFamily "JetBrains Mono"
Set FontSize 18
Set Width 1920
Set Height 1080
Set Framerate 25
Set Padding 24
Set LineHeight 1.16
Set WindowBar Colorful
Set WindowBarSize 40
Set BorderRadius 14
Set Margin 32
Set MarginFill "#11151c"
Set CursorBlink false
Set TypingSpeed 35ms
Set Theme { "name": "OperatorDeck", "black": "#1c2128", "red": "#f87171", "green": "#4ade80", "yellow": "#fbbf24", "blue": "#38bdf8", "magenta": "#c084fc", "cyan": "#66d9c4", "white": "#f0f6fc", "brightBlack": "#545d68", "brightRed": "#fca5a5", "brightGreen": "#86efac", "brightYellow": "#fde047", "brightBlue": "#8bb8ff", "brightMagenta": "#d8b4fe", "brightCyan": "#99f6e4", "brightWhite": "#ffffff", "background": "#0a0c10", "foreground": "#f0f6fc", "selectionBackground": "#2d333b", "cursor": "#ff891a" }

Hide
Type 'export PS1="\[\e[38;2;255;137;26m\]$(printf "\342\235\257")\[\e[0m\] "' Enter
Type "clear" Enter
Sleep 400ms
Show

# Quick CLI overview
Type "agy-swap list" Enter
Sleep 2.2s

# Launch interactive TUI
Type "agy-swap" Enter
Sleep 1.8s

# Inspect accounts across Pro and Ultra pools
Down
Sleep 1.3s
Down
Sleep 1.1s
Up
Sleep 800ms

# Switch active account
Enter
Sleep 1.1s
Enter
Sleep 2.0s

# Open Command Palette briefly
Ctrl+K
Sleep 900ms
Type "quota"
Sleep 1.3s
Escape
Sleep 400ms

# Clean exit
Type "q"
Sleep 400ms
