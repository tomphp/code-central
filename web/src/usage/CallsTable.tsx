// CallsTable is Usage's Code Central calls table: one row per operation,
// sortable by every column.
import type { UsageOperation } from "../api/types";
import { SortCue } from "../controls/Icon";
import { bytes, count } from "../format";

export interface Sort {
  key: keyof UsageOperation;
  descending: boolean;
}

export const DEFAULT_SORT: Sort = { key: "calls", descending: true };

// nextSort is the sort after a click on a column's header. A first click
// sorts numbers largest first and operations A to Z; a second reverses it.
export function nextSort(s: Sort, key: keyof UsageOperation): Sort {
  return s.key === key ? { key, descending: !s.descending } : { key, descending: key !== "op" };
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

export interface CallsTableProps {
  operations: UsageOperation[];
  sort: Sort;
  onSort(key: keyof UsageOperation): void;
}

// CallsTable is the Code Central calls table in the order sort gives; a
// click on a header calls onSort with its column.
export function CallsTable({ operations, sort, onSort }: CallsTableProps) {
  const rows = [...operations].sort((a, b) => {
    const x = a[sort.key];
    const y = b[sort.key];
    const order = typeof x === "string" ? x.localeCompare(y as string) : x - (y as number);
    return sort.descending ? -order : order;
  });

  return (
    <table aria-label="Code Central calls" className="data-table data-table-mono">
      <thead>
        <tr>
          {COLUMNS.map((c) => (
            <th
              key={c.key}
              scope="col"
              className={c.unit ? "num" : undefined}
              aria-sort={sort.key === c.key ? (sort.descending ? "descending" : "ascending") : undefined}
            >
              <button className="usage-sort" onClick={() => onSort(c.key)}>
                {c.label}
                <SortCue direction={sort.key === c.key ? (sort.descending ? "descending" : "ascending") : undefined} />
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
