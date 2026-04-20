export interface TocHeading {
  id: string;
  text: string;
  level: 2 | 3;
}

export interface DocNavItem {
  title: string;
  href: string;
  headings: TocHeading[];
}

export interface DocNavGroup {
  label: string;
  items: DocNavItem[];
}

export const DOCS_NAV: DocNavGroup[] = [
  {
    label: "Getting Started",
    items: [
      {
        title: "Guide",
        href: "/guide/",
        headings: [
          { id: "install", text: "Install", level: 2 },
          { id: "homebrew", text: "Homebrew", level: 3 },
          { id: "from-source", text: "From source", level: 3 },
          { id: "requirements", text: "Requirements", level: 3 },
          { id: "shell-integration-recommended", text: "Shell integration (recommended)", level: 2 },
          { id: "terminal-tab-titles-optional", text: "Terminal tab titles (optional)", level: 3 },
          { id: "claude-code-skill", text: "Claude Code skill", level: 2 },
          { id: "claude-code-status-tracking", text: "Claude Code status tracking", level: 2 },
          { id: "quick-start", text: "Quick start", level: 2 },
          { id: "tmux-integration", text: "Tmux integration", level: 2 },
          { id: "terminal-setup", text: "Terminal setup", level: 2 },
        ],
      },
    ],
  },
  {
    label: "Reference",
    items: [
      {
        title: "Commands",
        href: "/commands/",
        headings: [
          { id: "setup", text: "Setup", level: 2 },
          { id: "ww-clone-url-name", text: "ww clone", level: 3 },
          { id: "ww-shell-init-flags", text: "ww shell-init", level: 3 },
          { id: "worktrees", text: "Worktrees", level: 2 },
          { id: "ww-new-branch-flags", text: "ww new", level: 3 },
          { id: "ww-checkout-branch-or-pr-url-alias-co", text: "ww checkout", level: 3 },
          { id: "ww-sw", text: "ww sw", level: 3 },
          { id: "ww-rm-branch-flags", text: "ww rm", level: 3 },
          { id: "ww-ls-repo", text: "ww ls", level: 3 },
          { id: "stacks", text: "Stacks", level: 2 },
          { id: "stacked-prs", text: "Stacked PRs", level: 3 },
          { id: "ww-stack-status-alias-ww-stack-s", text: "ww stack status", level: 3 },
          { id: "ww-sync-branch", text: "ww sync", level: 3 },
          { id: "inspection", text: "Inspection", level: 2 },
          { id: "ww-status", text: "ww status", level: 3 },
          { id: "ww-dashboard-alias-dash-d", text: "ww dashboard", level: 3 },
          { id: "ww-log", text: "ww log", level: 3 },
          { id: "ww-gc", text: "ww gc", level: 3 },
          { id: "agents", text: "Agents", level: 2 },
          { id: "desktop-notifications", text: "Desktop notifications", level: 3 },
          { id: "ww-dispatch-prompt", text: "ww dispatch", level: 3 },
          { id: "ww-cc-setup", text: "ww cc-setup", level: 3 },
          { id: "agent-status", text: "Agent status", level: 3 },
          { id: "configuration", text: "Configuration", level: 2 },
          { id: "ww-config", text: "ww config", level: 3 },
          { id: "ww-doctor", text: "ww doctor", level: 3 },
          { id: "reference", text: "Reference", level: 2 },
          { id: "ww-tmux", text: "ww tmux", level: 3 },
          { id: "aliases", text: "Aliases", level: 3 },
          { id: "global-flags", text: "Global flags", level: 3 },
        ],
      },
      {
        title: "Tmux Integration",
        href: "/tmux/",
        headings: [
          { id: "setup", text: "Setup", level: 2 },
          { id: "picker-prefix--w", text: "Picker (prefix + w)", level: 2 },
          { id: "keybindings", text: "Keybindings", level: 3 },
          { id: "dispatch", text: "Dispatch", level: 3 },
          { id: "pr-picker", text: "PR picker", level: 3 },
          { id: "existing-branch-picker", text: "Existing branch picker", level: 3 },
          { id: "merged-worktree-indicator", text: "Merged worktree indicator", level: 3 },
          { id: "features", text: "Features", level: 3 },
          { id: "status-bar-widget", text: "Status bar widget", level: 2 },
          { id: "configuration", text: "Configuration", level: 3 },
          { id: "session-layout", text: "Session layout", level: 2 },
          { id: "per-pane-commands", text: "Per-pane commands", level: 3 },
          { id: "shell-integration", text: "Shell integration", level: 2 },
          { id: "commands-reference", text: "Commands reference", level: 2 },
          { id: "ww-tmux-pick", text: "ww tmux pick", level: 3 },
          { id: "ww-tmux-list", text: "ww tmux list", level: 3 },
          { id: "ww-tmux-status-bar", text: "ww tmux status-bar", level: 3 },
          { id: "ww-tmux-install", text: "ww tmux install", level: 3 },
        ],
      },
      {
        title: "Configuration",
        href: "/configuration/",
        headings: [
          { id: "config-file-locations", text: "Config file locations", level: 2 },
          { id: "config-schema", text: "Config schema", level: 2 },
          { id: "fields", text: "Fields", level: 3 },
          { id: "directory-structure", text: "Directory structure", level: 2 },
          { id: "willow-base-repos", text: "<willow-base>/repos/", level: 3 },
          { id: "willow-base-worktrees", text: "<willow-base>/worktrees/", level: 3 },
          { id: "willow-base-status", text: "<willow-base>/status/", level: 3 },
          { id: "claude-code-hooks", text: "Claude Code hooks", level: 3 },
        ],
      },
    ],
  },
];

const allPages = DOCS_NAV.flatMap((g) => g.items);

export function getPageHeadings(pathname: string): TocHeading[] {
  return allPages.find((p) => p.href === pathname)?.headings ?? [];
}

export function getPrevNext(currentPath: string) {
  const idx = allPages.findIndex((p) => p.href === currentPath);
  return {
    prev: idx > 0 ? allPages[idx - 1] : null,
    next: idx < allPages.length - 1 ? allPages[idx + 1] : null,
  };
}
