import { useId, useRef } from "react";
import type { KeyboardEvent, ReactNode } from "react";

// A controlled tab strip with the selected tab's panel (children) below it.
export interface TabsProps<T extends string> {
  label: string;
  tabs: readonly { value: T; label: string }[];
  value: T;
  onChange(value: T): void;
  // children is the selected tab's panel.
  children?: ReactNode;
}

export function Tabs<T extends string>({ label, tabs, value, onChange, children }: TabsProps<T>) {
  const id = useId();
  const refs = useRef(new Map<T, HTMLButtonElement>());
  const tabId = (v: T) => `${id}-tab-${v}`;

  function onKeyDown(e: KeyboardEvent) {
    const i = tabs.findIndex((t) => t.value === value);
    let next: number;
    switch (e.key) {
      case "ArrowLeft":
        next = (i - 1 + tabs.length) % tabs.length;
        break;
      case "ArrowRight":
        next = (i + 1) % tabs.length;
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = tabs.length - 1;
        break;
      default:
        return;
    }
    e.preventDefault();
    const to = tabs[next].value;
    onChange(to);
    refs.current.get(to)?.focus();
  }

  return (
    <div className="bs-tabs">
      <div role="tablist" aria-label={label} onKeyDown={onKeyDown}>
        {tabs.map((t) => (
          <button
            key={t.value}
            ref={(el) => {
              if (el) refs.current.set(t.value, el);
              else refs.current.delete(t.value);
            }}
            type="button"
            role="tab"
            id={tabId(t.value)}
            aria-selected={value === t.value}
            aria-controls={`${id}-panel`}
            tabIndex={value === t.value ? 0 : -1}
            onClick={() => onChange(t.value)}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div role="tabpanel" id={`${id}-panel`} aria-labelledby={tabId(value)}>
        {children}
      </div>
    </div>
  );
}
