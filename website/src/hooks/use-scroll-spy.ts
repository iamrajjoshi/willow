"use client";

import { useState, useEffect } from "react";

// Adapted from Docusaurus useTOCHighlight
// https://github.com/facebook/docusaurus/blob/main/packages/docusaurus-theme-common/src/hooks/useTOCHighlight.ts

function getActiveId(ids: string[], offset: number): string | null {
  const anchors = ids
    .map((id) => document.getElementById(id))
    .filter(Boolean) as HTMLElement[];

  if (anchors.length === 0) return null;

  // Find the first anchor whose top is at or below the offset line
  // (i.e. the next heading about to scroll into the active zone)
  const nextVisible = anchors.find(
    (el) => el.getBoundingClientRect().top >= offset,
  );

  if (nextVisible) {
    // If this anchor is in the top half of the viewport, it's the active one
    if (nextVisible.getBoundingClientRect().top < window.innerHeight / 2) {
      return nextVisible.id;
    }
    // Otherwise the content on screen belongs to the previous heading
    const idx = anchors.indexOf(nextVisible);
    return idx > 0 ? anchors[idx - 1].id : anchors[0].id;
  }

  // No anchor below the offset — we're at the bottom of the page
  return anchors[anchors.length - 1].id;
}

export function useScrollSpy(ids: string[], offset = 0) {
  const [activeId, setActiveId] = useState<string>(ids[0] ?? "");

  useEffect(() => {
    if (ids.length === 0) return;

    function update() {
      const id = getActiveId(ids, offset);
      if (id) setActiveId(id);
    }

    update();
    window.addEventListener("scroll", update, { passive: true });
    window.addEventListener("resize", update, { passive: true });
    return () => {
      window.removeEventListener("scroll", update);
      window.removeEventListener("resize", update);
    };
  }, [ids, offset]);

  return activeId;
}
