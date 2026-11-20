import type { CSSProperties } from "react";

// ribbonOf is a Project's ribbon colour, fixed by its Key, so a Project
// keeps the same one wherever it is shown.
export function ribbonOf(key: string): string {
  let h = 0;
  for (const c of key) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return `var(--ribbon-${(h % 6) + 1})`;
}

// ProjectRibbon is a Project's colour bar: decorative, the name beside it says which.
export function ProjectRibbon({ projectKey }: { projectKey: string }) {
  return <span className="ribbon" style={{ "--ribbon": ribbonOf(projectKey) } as CSSProperties} aria-hidden="true" />;
}
