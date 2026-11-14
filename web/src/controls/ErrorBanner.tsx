import type { ReactNode } from "react";

// ErrorBanner is the one error note: the global `.warning` style, announced as
// an alert. className adds only placement (flex-basis in a flex row), never size.
export function ErrorBanner({ children, id, className }: { children: ReactNode; id?: string; className?: string }) {
  return (
    <p role="alert" id={id} className={className ? `warning ${className}` : "warning"}>
      {children}
    </p>
  );
}
