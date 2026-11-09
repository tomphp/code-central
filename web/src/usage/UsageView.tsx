// Usage (Spec code-central-201): what agents have consumed in one
// Project, or in all of them, over the last 7, 30 or 90 days. For now it
// is the Code Central calls table: one row per MCP operation, sortable by
// each column. It loads when opened and whenever its scope or range
// changes, never live. Back returns to the view it was opened from.
import { useState } from "react";
import type { Api } from "../api/api";
import { USAGE_DAYS, type ProjectRow, type UsageDays, type UsageOperation } from "../api/types";
import { bytes, count } from "../format";
import { useFetch } from "../live";
import type { UiLocation } from "../url";
import "./usage.css";

export interface UsageViewProps {
  api: Api;
  // projects are every Project, Archived ones too, for the scope picker.
  projects?: ProjectRow[];
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function UsageView({ api, projects = [], loc, navigate }: UsageViewProps) {
  const days = loc.days ?? USAGE_DAYS[0];
  const usage = useFetch(() => api.usage({ project: loc.project, days }), [api, loc.project, days]);
  const back = loc.back?.view === "focus" ? "focus" : "inbox";
  const scopeName = loc.project ? (projects.find((p) => p.key === loc.project)?.display_name ?? loc.project) : "All Projects";
  // Rows answer the current scope and range only once they've loaded.
  const operations = usage.data?.operations;

  return (
    <main aria-label="Usage" className="usage">
      <header className="usage-head">
        <h1>Usage</h1>
        <label>
          <span className="usage-picker-label">Projects</span>
          <select value={loc.project ?? ""} onChange={(e) => navigate({ project: e.target.value || undefined })}>
            <option value="">All Projects</option>
            {projects.map((p) => (
              <option key={p.key} value={p.key}>
                {p.archived ? `${p.display_name} (Archived)` : p.display_name}
              </option>
            ))}
            {loc.project && !projects.some((p) => p.key === loc.project) && <option value={loc.project}>{loc.project}</option>}
          </select>
        </label>
        <label>
          <span className="usage-picker-label">Range</span>
          <select value={days} onChange={(e) => navigate({ days: Number(e.target.value) as UsageDays })}>
            {USAGE_DAYS.map((d) => (
              <option key={d} value={d}>
                Last {d} days
              </option>
            ))}
          </select>
        </label>
        <button className="btn usage-back" onClick={() => navigate(loc.back ?? { view: "inbox" })}>
          {back === "focus" ? "Back to Focus Mode" : "Back to the Inbox"}
        </button>
      </header>
      <div className="usage-body">
        {usage.error && (
          <p role="alert" className="warning">
            {usage.error.message}
          </p>
        )}
        <section aria-labelledby="usage-calls">
          <h2 id="usage-calls">Code Central calls</h2>
          {operations &&
            (operations.length ? (
              <CallsTable operations={operations} />
            ) : (
              <p className="muted">
                No usage yet in {scopeName} over the last {days} days.
              </p>
            ))}
        </section>
      </div>
    </main>
  );
}

// A Column is one column of the calls table: its header, how its cells
// read, and the unit their exact figure is given in on hover.
interface Column {
  key: keyof UsageOperation;
  label: string;
  unit?: "calls" | "bytes" | "errors";
}

const COLUMNS: Column[] = [
  { key: "op", label: "Operation" },
  { key: "calls", label: "Calls", unit: "calls" },
  { key: "request_bytes", label: "Request total", unit: "bytes" },
  { key: "average_request_bytes", label: "Request average", unit: "bytes" },
  { key: "response_bytes", label: "Response total", unit: "bytes" },
  { key: "average_response_bytes", label: "Response average", unit: "bytes" },
  { key: "errors", label: "Errors", unit: "errors" },
];

interface Sort {
  key: keyof UsageOperation;
  descending: boolean;
}

// CallsTable is the Code Central calls table, most calls first until the
// human sorts by another column. A first click sorts numbers largest
// first and operations A to Z; a second reverses it.
function CallsTable({ operations }: { operations: UsageOperation[] }) {
  const [sort, setSort] = useState<Sort>({ key: "calls", descending: true });
  const rows = [...operations].sort((a, b) => {
    const x = a[sort.key];
    const y = b[sort.key];
    const order = typeof x === "string" ? x.localeCompare(y as string) : x - (y as number);
    return sort.descending ? -order : order;
  });
  const sortBy = (key: keyof UsageOperation) =>
    setSort((s) => (s.key === key ? { key, descending: !s.descending } : { key, descending: key !== "op" }));

  return (
    <table aria-label="Code Central calls" className="usage-table">
      <thead>
        <tr>
          {COLUMNS.map((c) => (
            <th
              key={c.key}
              scope="col"
              className={c.unit ? "num" : undefined}
              aria-sort={sort.key === c.key ? (sort.descending ? "descending" : "ascending") : undefined}
            >
              <button className="usage-sort" onClick={() => sortBy(c.key)}>
                {c.label}
              </button>
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {rows.map((r) => (
          <tr key={r.op}>
            <th scope="row">{r.op}</th>
            {COLUMNS.slice(1).map((c) => {
              const n = r[c.key] as number;
              return (
                <td key={c.key} className="num" title={`${count(n)} ${c.unit}`}>
                  {c.unit === "bytes" ? bytes(n) : count(n)}
                </td>
              );
            })}
          </tr>
        ))}
      </tbody>
    </table>
  );
}
