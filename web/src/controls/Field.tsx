// A labelled form field with an optional hint under it. Give the control
// aria-describedby={hintId} so the hint is read with it.
import type { ReactNode } from "react";

export interface FieldProps {
  label: string;
  hint?: ReactNode;
  hintId?: string;
  children: ReactNode;
}

export function Field({ label, hint, hintId, children }: FieldProps) {
  return (
    <>
      <label className="project-options-field">
        {label} {children}
      </label>
      {hint !== undefined && (
        <p className="project-options-hint" id={hintId}>
          {hint}
        </p>
      )}
    </>
  );
}
