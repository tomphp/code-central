// The UI's icons, drawn in the transit diagram's own strokes: a station
// ring, a caret, a no-entry sign and three stops. Each is decorative,
// hidden from assistive technology; the control that holds it is named.
import type { ReactNode } from "react";

function Svg({ children, size = 14 }: { children: ReactNode; size?: number }) {
  return (
    <svg
      className="icon"
      width={size}
      height={size}
      viewBox="0 0 16 16"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {children}
    </svg>
  );
}

// Caret points down when open (or for a menu), right when closed.
export function Caret({ open = true, size = 12 }: { open?: boolean; size?: number }) {
  return <Svg size={size}>{open ? <path d="M4 6l4 4 4-4" /> : <path d="M6 4l4 4-4 4" />}</Svg>;
}

// Station is Focus Mode's mark: one station ring.
export function Station({ size = 14 }: { size?: number }) {
  return (
    <Svg size={size}>
      <circle cx="8" cy="8" r="5" strokeWidth="3" />
    </Svg>
  );
}

// More is a row's menu: three stops on a line.
export function More({ size = 14 }: { size?: number }) {
  return (
    <Svg size={size}>
      <path d="M2 8h12" strokeWidth="1.5" opacity=".45" />
      <circle cx="3.5" cy="8" r="1.6" fill="currentColor" stroke="none" />
      <circle cx="8" cy="8" r="1.6" fill="currentColor" stroke="none" />
      <circle cx="12.5" cy="8" r="1.6" fill="currentColor" stroke="none" />
    </Svg>
  );
}

// Blocked is a no-entry sign: a ring barred across.
export function Blocked({ size = 12 }: { size?: number }) {
  return (
    <Svg size={size}>
      <circle cx="8" cy="8" r="6" />
      <path d="M5 8h6" strokeWidth="2.5" />
    </Svg>
  );
}

// RequirementMark is a station on its line: a ring with the line running in and out.
export function RequirementMark({ size = 14 }: { size?: number }) {
  return (
    <Svg size={size}>
      <path d="M1 8h3M12 8h3" />
      <circle cx="8" cy="8" r="3.5" />
    </Svg>
  );
}

// MapMark is a node with two branches leaving it.
export function MapMark({ size = 14 }: { size?: number }) {
  return (
    <Svg size={size}>
      <path d="M5 8l6-4M5 8l6 4" />
      <circle cx="4" cy="8" r="2" fill="currentColor" />
      <circle cx="12" cy="4" r="1.2" fill="currentColor" />
      <circle cx="12" cy="12" r="1.2" fill="currentColor" />
    </Svg>
  );
}

// StopMark is a single stop: one filled dot, with no line.
export function StopMark({ size = 14 }: { size?: number }) {
  return (
    <Svg size={size}>
      <circle cx="8" cy="8" r="3" fill="currentColor" />
    </Svg>
  );
}

// Elbow marks a row as under its parent.
export function Elbow({ size = 12 }: { size?: number }) {
  return (
    <Svg size={size}>
      <path d="M4 2v7a2 2 0 002 2h6M9 8l3 3-3 3" />
    </Svg>
  );
}

// SortCue is a column's sort state: a caret up or down for the sorted
// column, both faint for a column that can be sorted.
export function SortCue({ direction, size = 12 }: { direction?: "ascending" | "descending"; size?: number }) {
  if (direction) return <Svg size={size}>{direction === "ascending" ? <path d="M4 10l4-4 4 4" /> : <path d="M4 6l4 4 4-4" />}</Svg>;
  return (
    <Svg size={size}>
      <path d="M4 6l4-3 4 3M4 10l4 3 4-3" opacity=".45" />
    </Svg>
  );
}
