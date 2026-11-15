import type { ReactNode } from "react";

// Empty is the one note for a view with nothing to show yet: "Loading…" while
// it loads, or what is missing, ending in a full stop.
export function Empty({ children }: { children: ReactNode }) {
  return <p className="empty">{children}</p>;
}

export function Loading() {
  return <Empty>Loading…</Empty>;
}
