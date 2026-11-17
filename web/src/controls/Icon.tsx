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
