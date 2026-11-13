// A dropdown of options. The optional placeholder is a first option with the
// empty value; aria-label, disabled and the other select attributes pass through.
import type { SelectHTMLAttributes } from "react";

export interface SelectProps<T extends string> extends Omit<
  SelectHTMLAttributes<HTMLSelectElement>,
  "value" | "onChange" | "children"
> {
  options: readonly { value: T; label?: string }[];
  value: T | "";
  onChange(value: T | ""): void;
  placeholder?: string;
}

export function Select<T extends string>({
  options,
  value,
  onChange,
  placeholder,
  ...rest
}: SelectProps<T>) {
  return (
    <select
      {...rest}
      value={value}
      onChange={(e) => onChange(e.target.value as T | "")}
    >
      {placeholder !== undefined && <option value="">{placeholder}</option>}
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label ?? o.value}
        </option>
      ))}
    </select>
  );
}
