# Tmux Integration

Willow includes native tmux support — a worktree picker popup, live Claude output preview, status bar widget, and automatic session management. If you use tmux, this replaces the need for any separate worktree-switching plugin.

## Setup

```bash
ww tmux install
```

This prints the tmux.conf lines to add. Copy them into your `~/.tmux.conf`:

```bash
bind w display-popup -E -w 90% -h 80% "/path/to/willow tmux pick"
set -g status-right '#(/path/to/willow tmux status-bar) %l:%M %a'
set -g status-interval 3
```

Then reload: `tmux source ~/.tmux.conf`

## Picker (`prefix + w`)

Press `prefix + w` to open the worktree picker in a popup:

<HeroTerminal gif="/demo-tmux-picker.gif" title="ww tmux pick" />

The right panel shows a live preview of the tmux pane content (Claude Code output).

### Keybindings

| Key | Action |
|-----|--------|
| `Enter` | Switch to worktree (creates tmux session if needed) |
| `Ctrl-N` | Create new worktree from typed query (also accepts PR URLs) |
| `Ctrl-E` | Browse existing remote branches and create a worktree |
| `Ctrl-P` | Browse open PRs and create a worktree for the selected one |
| `Ctrl-S` | Sync stacked worktrees (selected branch's subtree, or all) |
| `Ctrl-D` | Delete selected worktree and its tmux session |
| `Esc` | Close picker |

### PR picker

Press `Ctrl-P` to list open PRs via `gh pr list`. Each line shows the PR number, title, author, and branch. Select one to create a worktree (or switch to it if one already exists). Requires the [GitHub CLI](https://cli.github.com/).

You can also paste a PR URL into the query field and press `Ctrl-N` for quick one-off access.

### Existing branch picker

Press `Ctrl-E` to open a secondary picker showing all remote branches that don't already have worktrees. Select one to create a worktree and switch to it immediately.

### Merged worktree indicator

Worktrees whose branches have been merged into the base branch show a dim `[merged]` tag. These sort to the bottom of the list, making it easy to spot stale worktrees. Clean them up with `Ctrl-D` or `ww gc --prune`.

### Features

- **Auto-refresh** — the list reloads every 2 seconds with fresh agent status
- **Auto-navigate** — opens with the cursor on your current session
- **Status colors** — BUSY (green), WAIT (red), DONE (blue), IDLE (yellow)
- **Unread indicator** — `●` marks completed sessions you haven't viewed
- **Merged indicator** — `[merged]` marks worktrees whose branches are merged
- **Multi-Claude sub-rows** — when a worktree has multiple active Claude sessions, each is shown as an indented sub-row with its own status and tool info
- **Embedded fzf** — fzf is compiled into the willow binary, no external `fzf` dependency needed

## Status bar widget

The status bar shows worktree and active agent counts:

```
🌳 5 🤖 3
```

It also tracks state transitions — when a Claude session goes from BUSY to any other state, it triggers an audio notification (macOS `Glass.aiff` by default).

### Configuration

```jsonc
{
  "tmux": {
    "notification": true,           // enable/disable sound (default: true)
    "notifyCommand": "afplay /System/Library/Sounds/Glass.aiff"  // custom command
  }
}
```

Set `"notification": false` to disable sound.

## Session layout

By default, `willow tmux` creates a single window with one pane for each worktree session. You can customize this with `tmux.layout` — a list of raw tmux subcommands that run after session creation. The `-t` (target session) and `-c` (working directory) flags are auto-injected when not present.

```jsonc
{
  "tmux": {
    "layout": [
      "split-window -h",
      "select-layout even-horizontal"
    ]
  }
}
```

Any tmux subcommand works: `split-window`, `new-window`, `select-layout`, `resize-pane`, etc.

### Pane init commands

Use `tmux.postWorktreeCreate` to send shell commands to every pane after the layout is set up. These run once when a session is first created.

```jsonc
{
  "tmux": {
    "layout": [
      "split-window -h",
      "select-layout even-horizontal"
    ],
    "postWorktreeCreate": ["cd website"]
  }
}
```

This creates two side-by-side panes, each starting in the `website/` subdirectory.

## Shell integration

When inside tmux, `ww sw` automatically switches tmux sessions instead of just `cd`-ing. This works out of the box after running `eval "$(willow shell-init)"` — no additional setup needed.

| Context | `ww sw` behavior |
|---------|-----------------|
| Outside tmux | fzf picker → `cd` to worktree |
| Inside tmux | fzf picker → create/switch tmux session |

## Commands reference

### `ww tmux pick`

Interactive worktree picker. Designed to run inside a tmux popup.

```bash
ww tmux pick              # all repos
ww tmux pick -r myrepo    # filter to one repo
```

### `ww tmux list`

Print formatted picker lines. Used by fzf's reload binding for auto-refresh.

### `ww tmux status-bar`

Output tmux status-right widget string. Called every `status-interval` seconds.

### `ww tmux install`

Print the tmux.conf lines to add for willow integration.
