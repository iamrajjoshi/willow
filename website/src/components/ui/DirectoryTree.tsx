import { TerminalWindow } from "./TerminalWindow";

const TREE_LINES = [
  { structure: "<willow-base>/", dir: "", file: "", annotation: "" },
  {
    structure: "├── ",
    dir: "repos/",
    file: "",
    annotation: "bare clones (shared git database)",
  },
  { structure: "│   └── ", dir: "myrepo.git/", file: "", annotation: "" },
  {
    structure: "│       └── ",
    dir: "",
    file: "willow.json",
    annotation: "per-repo config",
  },
  {
    structure: "├── ",
    dir: "worktrees/",
    file: "",
    annotation: "isolated directories, one per branch",
  },
  { structure: "│   └── ", dir: "myrepo/", file: "", annotation: "" },
  { structure: "│       ├── ", dir: "main/", file: "", annotation: "" },
  {
    structure: "│       ├── ",
    dir: "auth-refactor/",
    file: "",
    annotation: "",
    badge: "busy" as const,
  },
  {
    structure: "│       └── ",
    dir: "payments/",
    file: "",
    annotation: "",
    badge: "wait" as const,
  },
  {
    structure: "└── ",
    dir: "status/",
    file: "",
    annotation: "Claude Code agent status",
  },
  { structure: "    └── ", dir: "myrepo/", file: "", annotation: "" },
  {
    structure: "        ├── ",
    dir: "",
    file: "auth-refactor.json",
    annotation: "",
  },
  {
    structure: "        └── ",
    dir: "",
    file: "payments.json",
    annotation: "",
  },
] as const;

const BADGE_STYLES = {
  busy: "bg-status-busy/[0.06] text-status-busy border-status-busy/[0.12]",
  wait: "bg-status-wait/[0.06] text-status-wait border-status-wait/[0.12]",
} as const;

export function DirectoryTree() {
  return (
    <TerminalWindow title="<willow-base>/">
      <div className="p-5 font-mono text-sm leading-[1.8]">
        {TREE_LINES.map((line, i) => (
          <div key={i} className="flex items-center whitespace-nowrap">
            <span className="text-willow-text-dim">{line.structure}</span>
            {line.dir && (
              <span className="font-semibold text-willow-accent">
                {line.dir}
              </span>
            )}
            {line.file && (
              <span className="text-willow-text-2">{line.file}</span>
            )}
            {"badge" in line && line.badge && (
              <span
                className={`ml-3 rounded-full border px-2 py-px text-[0.65rem] font-bold tracking-wider ${BADGE_STYLES[line.badge]}`}
              >
                {line.badge.toUpperCase()}
              </span>
            )}
            {line.annotation && (
              <span className="ml-4 hidden text-xs italic text-willow-text-3 sm:inline">
                {line.annotation}
              </span>
            )}
          </div>
        ))}
      </div>
    </TerminalWindow>
  );
}
