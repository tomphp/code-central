// TokenChart is Usage's token usage per day: one bar per day, oldest
// first, stacked by input, cache write, cache read and output, drawn as
// hand-written SVG (no chart library). A kind filter picks the kinds of
// session counted, from the days already read. Hovering or focusing a bar
// shows its exact figures. Tokens only: never a price.
import { useState } from "react";
import type { UsageDay, UsageDayTokens } from "../api/types";
import { count } from "../format";

export interface TokenChartProps {
  days: UsageDay[];
  // kinds are the kinds of session in the range, in their order.
  kinds: string[];
}

// TOKEN_TYPES are the stack's layers, bottom first.
const TOKEN_TYPES = [
  { key: "input_tokens", label: "Input", className: "token-input" },
  { key: "cache_write_tokens", label: "Cache write", className: "token-cache-write" },
  { key: "cache_read_tokens", label: "Cache read", className: "token-cache-read" },
  { key: "output_tokens", label: "Output", className: "token-output" },
] as const;

type TokenType = (typeof TOKEN_TYPES)[number]["key"];
type Totals = Record<TokenType, number>;

// The chart's drawing area, in SVG units: the axis labels sit in the
// left and bottom margins.
const WIDTH = 900;
const HEIGHT = 220;
const LEFT = 56;
const BOTTOM = 22;
const TOP = 8;

export function TokenChart({ days, kinds }: TokenChartProps) {
  const [kind, setKind] = useState("");
  const [shown, setShown] = useState<number>();
  const rows = days.map((d) => ({ day: d.day, totals: totals(d.kinds.filter((k) => !kind || k.kind === kind)) }));
  const top = niceMax(Math.max(0, ...rows.map((r) => sum(r.totals))));
  const plotHeight = HEIGHT - TOP - BOTTOM;
  const slot = (WIDTH - LEFT) / rows.length;
  const barWidth = Math.max(1, slot * 0.7);
  const y = (n: number) => TOP + plotHeight - (top ? (n / top) * plotHeight : 0);
  // Label about seven days along the axis, whatever the range.
  const every = Math.ceil(rows.length / 7);
  const focused = shown === undefined ? undefined : rows[shown];

  return (
    <>
      <div className="token-controls">
        <label>
          <span className="usage-picker-label">Kind</span>
          <select value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="">All kinds</option>
            {kinds.map((k) => (
              <option key={k} value={k}>
                {k}
              </option>
            ))}
          </select>
        </label>
        <ul className="token-legend">
          {TOKEN_TYPES.map((t) => (
            <li key={t.key}>
              <span className={`token-swatch ${t.className}`} aria-hidden="true" />
              {t.label}
            </li>
          ))}
        </ul>
      </div>
      <svg role="group" aria-label="Token usage per day" className="token-chart" viewBox={`0 0 ${WIDTH} ${HEIGHT}`} onMouseLeave={() => setShown(undefined)}>
        {(top ? [0, top / 2, top] : [0]).map((n) => (
          <g key={n} className="token-grid" aria-hidden="true">
            <line x1={LEFT} x2={WIDTH} y1={y(n)} y2={y(n)} />
            <text x={LEFT - 6} y={y(n)} dy="0.32em" textAnchor="end">
              {compact(n)}
            </text>
          </g>
        ))}
        {rows.map((r, i) => {
          const x = LEFT + i * slot + (slot - barWidth) / 2;
          let base = 0;
          return (
            <g
              key={r.day}
              role="img"
              aria-label={`${dayLabel(r.day)}: ${count(sum(r.totals))} tokens`}
              tabIndex={0}
              className={shown === i ? "token-bar shown" : "token-bar"}
              onMouseEnter={() => setShown(i)}
              onFocus={() => setShown(i)}
              onBlur={() => setShown(undefined)}
            >
              {/* The whole slot takes the hover, so a short bar is easy to reach. */}
              <rect className="token-hit" x={LEFT + i * slot} y={TOP} width={slot} height={plotHeight} />
              {TOKEN_TYPES.map((t) => {
                const n = r.totals[t.key];
                const rect = <rect key={t.key} className={t.className} x={x} y={y(base + n)} width={barWidth} height={y(base) - y(base + n)} />;
                base += n;
                return rect;
              })}
              {i % every === (rows.length - 1) % every && (
                <text className="token-day" x={x + barWidth / 2} y={HEIGHT - 6} textAnchor="middle">
                  {dayLabel(r.day)}
                </text>
              )}
            </g>
          );
        })}
      </svg>
      <div role="status" aria-label="Day's figures" className="token-figures">
        {focused ? (
          <>
            <strong>{dayLabel(focused.day)}</strong>
            <dl>
              {TOKEN_TYPES.map((t) => (
                <div key={t.key}>
                  <dt>{t.label}</dt>
                  <dd>{count(focused.totals[t.key])}</dd>
                </div>
              ))}
              <div>
                <dt>Total</dt>
                <dd>{count(sum(focused.totals))}</dd>
              </div>
            </dl>
          </>
        ) : (
          <span className="muted">Point at a bar for its day's figures.</span>
        )}
      </div>
    </>
  );
}

// totals sums kinds' tokens by token type.
function totals(kinds: UsageDayTokens[]): Totals {
  const t: Totals = { input_tokens: 0, cache_write_tokens: 0, cache_read_tokens: 0, output_tokens: 0 };
  for (const k of kinds) for (const { key } of TOKEN_TYPES) t[key] += k[key];
  return t;
}

function sum(t: Totals): number {
  return t.input_tokens + t.cache_write_tokens + t.cache_read_tokens + t.output_tokens;
}

// niceMax is the axis's top: n rounded up to 1, 2 or 5 times a power of
// ten, so its half is a round figure too.
function niceMax(n: number): number {
  if (n <= 0) return 0;
  const p = 10 ** Math.floor(Math.log10(n));
  return [1, 2, 5, 10].map((m) => m * p).find((m) => m >= n) ?? 10 * p;
}

// compact is an axis label: 0, 500, 1.5K, 2M.
function compact(n: number): string {
  if (n >= 1_000_000) return `${Math.round(n / 100_000) / 10}M`;
  if (n >= 1_000) return `${Math.round(n / 100) / 10}K`;
  return String(Math.round(n));
}

// dayLabel is a YYYY-MM-DD day as the locale puts it, short: "9 Nov".
function dayLabel(day: string): string {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString([], { day: "numeric", month: "short" });
}
