import { stateSlug } from "./StateChip";

// KindBadge is a Spec's kind (Map, Long-lived, Spec) as a badge.
export function KindBadge({ kind }: { kind: string }) {
  return <span className={`kind-badge kind-${stateSlug(kind)}`}>{kind}</span>;
}
