// The Limits popup (Spec code-central-861): the selected Project's Plan
// Limits as a meter per window, fixed at the bottom centre of every view.
// Its countdowns, resets and age note come from the UI's clock, which it
// reads again each minute without a refetch.
import { useEffect, useState } from "react";
import type { Options, PlanLimits, PlanLimitWindow, ProjectRow } from "../api/types";
import { ago } from "../format";
import "./limits.css";

const MINUTE = 60_000;

// STALE_MS is how old figures are before the popup says when they were read.
const STALE_MS = 10 * MINUTE;

export interface LimitsPopupProps {
  // project is the selected Project; none under All Projects.
  project?: ProjectRow;
  // option is the Show Limits popup Option; none until the Options load.
  option?: Options["limits_popup"];
  now: () => Date;
}

// LimitsPopup renders nothing unless the Option is on and the selected
// Project's Factory Status has Plan Limits with at least one window.
export function LimitsPopup({ project, option, now }: LimitsPopupProps) {
  const limits = project?.factory_status.plan_limits;
  if (option !== "on" || !limits || (!limits.five_hour && !limits.weekly)) return null;
  return <Popup limits={limits} now={now} />;
}

// Popup is the shown popup, rendered again each minute.
function Popup({ limits, now }: { limits: PlanLimits; now: () => Date }) {
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((t) => t + 1), MINUTE);
    return () => clearInterval(id);
  }, []);
  const at = now();
  const stale = at.getTime() - Date.parse(limits.observed_at) > STALE_MS;
  return (
    <section className="limits-popup" aria-label="Limits">
      {limits.five_hour && <Meter name="5-hour" limit={limits.five_hour} now={at} />}
      {limits.weekly && <Meter name="Weekly" limit={limits.weekly} now={at} />}
      {stale && <span className="limits-age">updated {ago(at, limits.observed_at)}</span>}
    </section>
  );
}

// Meter is one window: its bar, its whole-number percentage and when it
// resets; once that has passed, 0% and "reset".
function Meter({ name, limit, now }: { name: string; limit: PlanLimitWindow; now: Date }) {
  const left = Date.parse(limit.resets_at) - now.getTime();
  const percent = left > 0 ? Math.round(limit.used_percent) : 0;
  return (
    <div className="limits-window">
      <span className="limits-name">{name}</span>
      <div
        role="meter"
        aria-label={name}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={percent}
        aria-valuetext={`${percent}%`}
        className={`limits-bar ${colour(percent)}`}
      >
        <div className="limits-fill" style={{ width: `${percent}%` }} />
      </div>
      <span className="limits-percent">{percent}%</span>
      <span className="limits-reset">{left > 0 ? `resets in ${countdown(left)}` : "reset"}</span>
    </div>
  );
}

// colour is the bar's class: amber from 75%, red from 90%.
function colour(percent: number): string {
  if (percent >= 90) return "red";
  if (percent >= 75) return "amber";
  return "";
}

// countdown is ms ahead in whole minutes, rounded up: "14m", "2h 14m", or
// past a day "3d 4h".
function countdown(ms: number): string {
  const m = Math.ceil(ms / MINUTE);
  const h = Math.floor(m / 60);
  const d = Math.floor(h / 24);
  if (d > 0) return `${d}d ${h % 24}h`;
  if (h > 0) return `${h}h ${m % 60}m`;
  return `${m}m`;
}
