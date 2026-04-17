Output screenshots/demo-dashboard.gif

Set Shell "zsh"
Set FontSize 16
Set Width 1100
Set Height 600
Set Theme { "name": "Willow Dark", "background": "#0a0a0f", "foreground": "#fafafa", "selectionBackground": "#5eead4", "cursorColor": "#5eead4", "black": "#3f3f46", "red": "#f87171", "green": "#5eead4", "yellow": "#fbbf24", "blue": "#5eead4", "purple": "#5eead4", "cyan": "#5eead4", "white": "#fafafa", "brightBlack": "#71717a", "brightRed": "#f87171", "brightGreen": "#5eead4", "brightYellow": "#fbbf24", "brightBlue": "#5eead4", "brightPurple": "#5eead4", "brightCyan": "#5eead4", "brightWhite": "#fafafa" }
Set Padding 20
Set TypingSpeed 50ms

Hide
Type "unset PROMPT_COMMAND; export PS1='$ '"
Enter
Type "source /Users/raj.joshi/.willow/worktrees/willow/raj--chore--audit-cleanup/screenshots/seed-demo-sandbox.sh"
Enter
Sleep 4s
Type "alias willow=/Users/raj.joshi/.willow/worktrees/willow/raj--chore--audit-cleanup/bin/willow"
Enter
Type "cd /tmp/wwdemo/.willow/worktrees/willow/main"
Enter
Type "clear"
Enter
Sleep 500ms
Show

Type "willow dashboard"
Enter
Sleep 1s
Sleep 4s
Ctrl+C
Sleep 1s
