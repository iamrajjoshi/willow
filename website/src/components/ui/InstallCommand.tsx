"use client";

import { CopyButton } from "./CopyButton";

const INSTALL_CMD = "brew install iamrajjoshi/tap/willow";

export function InstallCommand() {
  return (
    <div className="inline-flex items-center gap-3 rounded-full border border-willow-border bg-willow-bg-soft px-5 py-2.5">
      <span className="font-mono text-sm text-willow-text-2">
        $ {INSTALL_CMD}
      </span>
      <CopyButton text={INSTALL_CMD} />
    </div>
  );
}
