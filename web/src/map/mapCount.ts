import type { MapView } from "../api/types";

// mapCount is a Map's progress as a count: "<decided>/<total> decided",
// then "· <n> needs you" only when something does.
export function mapCount(m: MapView): string {
  const decided = `${m.decided}/${m.total} decided`;
  return m.needs_you > 0 ? `${decided} · ${m.needs_you} needs you` : decided;
}
