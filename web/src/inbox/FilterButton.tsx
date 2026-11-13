export function FilterButton(p: {
  on: boolean;
  label: string;
  count: number;
  loud?: boolean;
  sub?: boolean;
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
      {p.label}{" "}
      <span className={p.loud && p.count > 0 ? "count loud" : "count"}>
        {p.count}
      </span>
    </button>
  );
}
