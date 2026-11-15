// A labelled group of buttons, one of them pressed.
export interface ToggleGroupProps<T extends string> {
  label: string;
  options: readonly { value: T; label: string }[];
  value: T;
  onChange(value: T): void;
  // disabled shows the group read-only.
  disabled?: boolean;
}

export function ToggleGroup<T extends string>({ label, options, value, onChange, disabled }: ToggleGroupProps<T>) {
  return (
    <div className="bs-toggle" role="group" aria-label={label}>
      {options.map((o) => (
        <button key={o.value} type="button" aria-pressed={value === o.value} disabled={disabled} onClick={() => onChange(o.value)}>
          {o.label}
        </button>
      ))}
    </div>
  );
}
