import { Count } from "../controls/Count";
import { stateSlug } from "../controls/StateChip";

export function FilterButton(p: {
  on: boolean;
  label: string;
  count: number;
  loud?: boolean;
  sub?: boolean;
  // station is a State, drawn as a ring in its line's colour before the label.
  station?: string;
  onClick(): void;
}) {
  return (
    <button
      className={["filter", p.on && "on", p.sub && "sub"]
        .filter(Boolean)
        .join(" ")}
      aria-pressed={p.on}
      onClick={p.onClick}
    >
      <span className="filter-label">
        {p.station && <span className={`station st-${stateSlug(p.station)}`} aria-hidden="true" />}
        {p.label}
      </span>{" "}
      <Count n={p.count} loud={p.loud && p.count > 0} />
    </button>
  );
}
