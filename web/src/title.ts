// The tab shows the Needs Me count in its title and favicon, so a
// background tab still says when something waits on the human.
import { useEffect } from "react";

export function faviconFor(count: number): string {
  const badge =
    count > 0
      ? `<circle cx="22" cy="10" r="10" fill="#d97706"/><text x="22" y="14.5" font-family="system-ui,sans-serif" font-size="13" font-weight="700" fill="#fff" text-anchor="middle">${count > 99 ? "99+" : count}</text>`
      : "";
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32"><rect x="2" y="6" width="24" height="24" rx="6" fill="#2f5bd3"/>${badge}</svg>`;
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}

export function useNeedsMeTitle(count: number | undefined) {
  useEffect(() => {
    if (count === undefined) return;
    document.title = count > 0 ? `(${count}) Code Central` : "Code Central";
    let link = document.querySelector<HTMLLinkElement>("link[rel=icon]");
    if (!link) {
      link = document.createElement("link");
      link.rel = "icon";
      document.head.appendChild(link);
    }
    link.href = faviconFor(count);
  }, [count]);
}
