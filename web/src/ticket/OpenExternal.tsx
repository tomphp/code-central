// Open in GitHub (Spec code-central-279): a linked Spec's or Ticket's
// link to its external item, as its Link's URL, so the human can read it,
// its comments and its reporter there. It stays once the Link is
// detached, for history.
import type { LinkView } from "../api/types";

// INTEGRATIONS are the names Integrations go by, by the name their Links
// carry; any other goes by that name.
const INTEGRATIONS: Record<string, string> = { github: "GitHub" };

export function OpenExternal({ link }: { link?: LinkView }) {
  if (!link?.url) return null;
  return (
    <a className="btn open-external" href={link.url} target="_blank" rel="noreferrer">
      Open in {INTEGRATIONS[link.integration] ?? link.integration}
    </a>
  );
}
