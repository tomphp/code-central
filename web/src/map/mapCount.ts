// A Map's decision count, as its Spec row's headline and its pane show it.
import type { MapView } from "../api/types";

// mapCount is a Map's progress as a count: "<decided>/<total> decided",
// then "· <n> needs you" only when something does.
export function mapCount(m: MapView): string {
  const decided = `${m.decided}/${m.total} decided`;
  return m.needs_you > 0 ? `${decided} · ${m.needs_you} needs you` : decided;
}
