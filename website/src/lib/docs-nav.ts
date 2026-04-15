export interface DocNavItem {
  title: string;
  href: string;
}

export interface DocNavGroup {
  label: string;
  items: DocNavItem[];
}

export const DOCS_NAV: DocNavGroup[] = [
  {
    label: "Getting Started",
    items: [{ title: "Guide", href: "/guide/" }],
  },
  {
    label: "Reference",
    items: [
      { title: "Commands", href: "/commands/" },
      { title: "Tmux Integration", href: "/tmux/" },
      { title: "Configuration", href: "/configuration/" },
    ],
  },
];

const allPages = DOCS_NAV.flatMap((g) => g.items);

export function getPrevNext(currentPath: string) {
  const idx = allPages.findIndex((p) => p.href === currentPath);
  return {
    prev: idx > 0 ? allPages[idx - 1] : null,
    next: idx < allPages.length - 1 ? allPages[idx + 1] : null,
  };
}
