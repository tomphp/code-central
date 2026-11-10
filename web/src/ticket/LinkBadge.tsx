// The Link badge (ticket 42): a linked item's Integration and external ID,
// marked when unlinked, or when outbound (code-central-290): Code Central
// owns the item and only writes to the external one. In the detail it
// links to the external item; on a row, which is itself a link, it is
// plain text with the URL as its tooltip.
import type { LinkView } from "../api/types";

export function linkName(l: LinkView): string {
  return `${l.integration} ${l.id}`;
}

export function LinkBadge({ link: l, plain }: { link: LinkView; plain?: boolean }) {
  const text = linkName(l) + (l.detached ? " (unlinked)" : l.outbound ? " (outbound)" : "");
  const cls = `label link-badge${l.detached ? " detached" : ""}`;
  if (plain || !l.url) {
    return (
      <span className={cls} title={l.url}>
        {text}
      </span>
    );
  }
  return (
    <a className={cls} href={l.url} target="_blank" rel="noreferrer">
      {text}
    </a>
  );
}
