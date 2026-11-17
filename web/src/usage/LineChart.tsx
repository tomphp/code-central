// LineChart is a hand-written SVG line chart (Spec code-central-201: no
// chart library): one value per point, in order along the x axis, from
// zero up the y axis. Each point is focusable and, hovered or focused,
// shows the tooltip tip gives it.
import { useState, type ReactNode } from "react";
import { count } from "../format";

export interface LineChartProps {
  // label names the chart for a screen reader.
  label: string;
  values: number[];
  // pointLabel names point i for a screen reader.
  pointLabel(i: number): string;
  // tip is point i's tooltip.
  tip(i: number): ReactNode;
  // xLabel names the x axis.
  xLabel: string;
}

// The chart's drawing size, in SVG units, and the margins its axes'
// labels sit in.
const WIDTH = 720;
const HEIGHT = 240;
const LEFT = 64;
const RIGHT = 16;
const TOP = 12;
const BOTTOM = 36;
const TICKS = 4;

export function LineChart({ label, values, pointLabel, tip, xLabel }: LineChartProps) {
  const [active, setActive] = useState<number>();
  const top = niceCeiling(Math.max(...values, 1));
  const plotW = WIDTH - LEFT - RIGHT;
  const plotH = HEIGHT - TOP - BOTTOM;
  const x = (i: number) => LEFT + (values.length > 1 ? (i / (values.length - 1)) * plotW : plotW / 2);
  const y = (v: number) => TOP + plotH - (v / top) * plotH;
  const ticks = Array.from({ length: TICKS + 1 }, (_, i) => (top / TICKS) * i);
  const xTicks = xTickIndexes(values.length);
  const peak = values.indexOf(Math.max(...values));

  return (
    <div className="line-chart">
      <svg role="img" aria-label={label} viewBox={`0 0 ${WIDTH} ${HEIGHT}`}>
        {ticks.map((t) => (
          <g key={t} className="line-chart-grid">
            <line x1={LEFT} x2={WIDTH - RIGHT} y1={y(t)} y2={y(t)} />
            <text x={LEFT - 8} y={y(t)} textAnchor="end" dominantBaseline="middle">
              {count(t)}
            </text>
          </g>
        ))}
        {xTicks.map((i) => (
          <text key={i} className="line-chart-axis" x={x(i)} y={HEIGHT - BOTTOM + 16} textAnchor="middle">
            {i + 1}
          </text>
        ))}
        <text className="line-chart-axis" x={LEFT + plotW / 2} y={HEIGHT - 4} textAnchor="middle">
          {xLabel}
        </text>
        <polyline className="line-chart-line" points={values.map((v, i) => `${x(i)},${y(v)}`).join(" ")} />
        {values.map((v, i) => (
          <circle
            key={i}
            role="button"
            tabIndex={0}
            aria-label={pointLabel(i)}
            className={i === active ? "line-chart-point active" : "line-chart-point"}
            cx={x(i)}
            cy={y(v)}
            r={i === active ? 5 : 3.5}
            onMouseEnter={() => setActive(i)}
            onMouseLeave={() => setActive(undefined)}
            onFocus={() => setActive(i)}
            onBlur={() => setActive(undefined)}
          />
        ))}
        <text className="line-chart-peak" x={x(peak)} y={y(values[peak]) - 10} textAnchor={peakAnchor(x(peak))}>
          {count(values[peak])}
        </text>
      </svg>
      {active !== undefined && (
        <div
          role="tooltip"
          // Below a point in the upper half, so the popup it sits in doesn't clip it.
          className={y(values[active]) < HEIGHT / 2 ? "line-chart-tip below" : "line-chart-tip"}
          style={{ left: `${(x(active) / WIDTH) * 100}%`, top: `${(y(values[active]) / HEIGHT) * 100}%` }}
        >
          {tip(active)}
        </div>
      )}
    </div>
  );
}

// MAX_X_TICKS is how many x-axis ticks the chart labels at most.
const MAX_X_TICKS = 8;

// xTickIndexes are the points the x axis labels, by one rule: every point
// up to MAX_X_TICKS, then MAX_X_TICKS evenly spaced, first and last among them.
export function xTickIndexes(n: number): number[] {
  if (n <= MAX_X_TICKS) return Array.from({ length: n }, (_, i) => i);
  return [...new Set(Array.from({ length: MAX_X_TICKS }, (_, k) => Math.round((k * (n - 1)) / (MAX_X_TICKS - 1))))];
}

// peakAnchor keeps the peak's label inside the chart at either end.
function peakAnchor(px: number): "start" | "middle" | "end" {
  if (px < LEFT + 24) return "start";
  if (px > WIDTH - RIGHT - 24) return "end";
  return "middle";
}

// niceCeiling is the smallest of 1, 2, 2.5 or 5 times a power of ten, at
// least 1, times the TICKS, at or above n, so the y axis's ticks read as
// round whole figures.
function niceCeiling(n: number): number {
  const step = Math.max(n / TICKS, 1);
  const power = 10 ** Math.floor(Math.log10(step));
  const nice = [1, 2, 2.5, 5, 10].find((m) => m * power >= step) ?? 10;
  return nice * power * TICKS;
}
