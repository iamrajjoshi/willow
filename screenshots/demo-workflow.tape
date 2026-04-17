Output screenshots/demo-workflow.gif

Set Shell "zsh"
Set FontSize 16
Set Width 1100
Set Height 600
Set Theme { "name": "Willow Dark", "background": "#0a0a0f", "foreground": "#fafafa", "selectionBackground": "#5eead4", "cursorColor": "#5eead4", "black": "#3f3f46", "red": "#f87171", "green": "#5eead4", "yellow": "#fbbf24", "blue": "#5eead4", "purple": "#5eead4", "cyan": "#5eead4", "white": "#fafafa", "brightBlack": "#71717a", "brightRed": "#f87171", "brightGreen": "#5eead4", "brightYellow": "#fbbf24", "brightBlue": "#5eead4", "brightPurple": "#5eead4", "brightCyan": "#5eead4", "brightWhite": "#fafafa" }
Set Padding 20
Set TypingSpeed 50ms

Hide
Type "alias willow=/Users/raj/code/willow/bin/willow"
Enter
Type "cd ~/.willow/worktrees/willow/main"
Enter
Sleep 500ms
Type "clear"
Enter
Sleep 300ms
Show

Type "willow new feat/search --no-fetch"
Sleep 300ms
Enter
Sleep 2s

Type "willow ls"
Sleep 300ms
Enter
Sleep 2s

Type "willow status"
Sleep 300ms
Enter
Sleep 2s

Type "willow rm feat/search"
Sleep 300ms
Enter
Sleep 2s
