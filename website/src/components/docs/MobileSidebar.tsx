"use client";

import { useState } from "react";
import { Menu, X } from "lucide-react";
import { Sidebar } from "./Sidebar";

export function MobileSidebar() {
  const [open, setOpen] = useState(false);

  return (
    <>
      <button
        onClick={() => setOpen(true)}
        className="text-willow-text-2 lg:hidden"
      >
        <Menu className="h-5 w-5" />
      </button>

      {open && (
        <>
          <div
            className="fixed inset-0 z-40 bg-black/50 lg:hidden"
            onClick={() => setOpen(false)}
          />
          <div className="fixed inset-y-0 left-0 z-50 w-72 bg-willow-bg border-r border-willow-border p-6 pt-20 lg:hidden overflow-y-auto">
            <button
              onClick={() => setOpen(false)}
              className="absolute top-5 right-5 text-willow-text-3"
            >
              <X className="h-5 w-5" />
            </button>
            <Sidebar />
          </div>
        </>
      )}
    </>
  );
}
