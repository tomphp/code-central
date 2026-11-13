// Pulses: a brief scale-and-glow that marks something on the Activity
// Dashboard as just changed. A new trigger restarts the animation, and
// triggers arriving mid-animation don't queue.
import { useEffect, useState } from "react";

// pulseTiming.ms is how long a pulse lasts; focus.css's animations match
// it. board is the Board view's longer one, its card glow run twice, as
// board.css's animation. Tests lengthen both, so a slow machine can't end a
// pulse before they look for it.
export const pulseTiming = { ms: 600, board: 2800 };

// Pulse is what a pulsing element spreads on itself: data-pulsing marks
// that it is pulsing, and its class adds the one that runs the animation.
export interface Pulse {
  "data-pulsing"?: "true";
  className: string;
}

// usePulse gives an element of className its Pulse: it pulses whenever
// trigger differs from the value it last saw, but not on its first
// value, unless onMount says to pulse once then. lasts names the
// pulseTiming it lasts for.
export function usePulse(className: string, trigger: unknown, onMount = false, lasts: keyof typeof pulseTiming = "ms"): Pulse {
  const [seen, setSeen] = useState(trigger);
  // count is how many pulses have started; its parity picks between two
  // identical animations, so a new pulse swaps the animation's name and
  // the browser restarts it.
  const [count, setCount] = useState(onMount ? 1 : 0);
  const [on, setOn] = useState(onMount);
  // Set while rendering, so the pulse starts with the change it marks.
  if (!Object.is(seen, trigger)) {
    setSeen(trigger);
    setCount(count + 1);
    setOn(true);
  }
  useEffect(() => {
    if (!on) return;
    const timer = setTimeout(() => setOn(false), pulseTiming[lasts]);
    return () => clearTimeout(timer);
  }, [on, count, lasts]);
  return on ? { "data-pulsing": "true", className: `${className} dashboard-pulse-${count % 2}` } : { className };
}
