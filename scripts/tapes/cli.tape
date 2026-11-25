Output "docs/assets/cli.mp4"

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
Type 'export PS1="\[\e[38;2;255;137;26m\]$(printf "\342\235\257")\[\e[0m\] "' Enter
Type "clear" Enter
Sleep 400ms
Show

# Shot 1: full account and quota overview
Type "agy-swap list" Enter
Sleep 3.0s

# Clean transition before script commands
Hide
Type "clear" Enter
Sleep 200ms
Show

# Shot 2: status, recommend, switch, and statusline
Type "agy-swap status" Enter
Sleep 1.6s

Type "agy-swap recommend" Enter
Sleep 2.4s

Type "agy-swap switch beta@example.invalid" Enter
Sleep 1.8s

Type "agy-swap statusline" Enter
Sleep 2.2s
