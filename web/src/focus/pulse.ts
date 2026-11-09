// Pulses: a brief scale-and-glow that marks something on the Activity
// Dashboard as just changed. A new trigger restarts the animation, and
// triggers arriving mid-animation don't queue.
import { useEffect, useState } from "react";

// PULSE_MS is how long a pulse lasts; focus.css's animations match it.
const PULSE_MS = 600;

// Pulse is what a pulsing element spreads on itself: data-pulsing marks
// that it is pulsing, and its class adds the one that runs the animation.
export interface Pulse {
  "data-pulsing"?: "true";
  className: string;
}

// usePulse gives an element of className its Pulse: it pulses whenever
// trigger differs from the value it last saw, but not on its first
// value, unless onMount says to pulse once then.
export function usePulse(className: string, trigger: unknown, onMount = false): Pulse {
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
    const timer = setTimeout(() => setOn(false), PULSE_MS);
    return () => clearTimeout(timer);
  }, [on, count]);
  return on ? { "data-pulsing": "true", className: `${className} dashboard-pulse-${count % 2}` } : { className };
}
