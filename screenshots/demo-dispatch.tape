Output screenshots/demo-dispatch.gif

Set Shell "zsh"
Set FontSize 16
Set Width 1100
Set Height 500
Set Theme { "name": "Willow Dark", "background": "#0a0a0f", "foreground": "#fafafa", "selectionBackground": "#5eead4", "cursorColor": "#5eead4", "black": "#3f3f46", "red": "#f87171", "green": "#5eead4", "yellow": "#fbbf24", "blue": "#5eead4", "purple": "#5eead4", "cyan": "#5eead4", "white": "#fafafa", "brightBlack": "#71717a", "brightRed": "#f87171", "brightGreen": "#5eead4", "brightYellow": "#fbbf24", "brightBlue": "#5eead4", "brightPurple": "#5eead4", "brightCyan": "#5eead4", "brightWhite": "#fafafa" }
Set Padding 20
Set TypingSpeed 50ms

Hide
Type "unset PROMPT_COMMAND; export PS1='$ '"
Enter
Type "source ${WILLOW_BIN%/bin/willow}/screenshots/seed-demo-sandbox.sh"
Enter
Sleep 4s
Type "alias willow=$WILLOW_BIN"
Enter
Type "claude() { echo; echo 'claude code launched with prompt: '$* ; }"
Enter
Type "cd /tmp/wwdemo/.willow/worktrees/willow/main"
Enter
Type "clear"
Enter
Sleep 500ms
Show

Type `willow dispatch "add retry logic to the fetcher" --no-fetch`
Sleep 300ms
Enter
Sleep 3s

Type "willow ls"
Sleep 300ms
Enter
Sleep 3s
