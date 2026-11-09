// The Options view: the human's global preferences, each saved to the
// service the moment it's chosen. Every Option is one entry in OPTIONS, so
// a later one is another entry there. Leaving returns to the view the
// human came from, as the URL's from says.
import { useEffect, useState } from "react";
import type { Api } from "../api/api";
import type { ApiError, Options } from "../api/types";
import { returnViewOf, type UiLocation } from "../url";
import "./options.css";

// An OptionChoice is one value an Option can take, as the human reads it.
interface OptionChoice<V> {
  value: V;
  label: string;
  hint: string;
}

// An OptionDefinition is how the view offers one Option: its label and
// every value it can take.
interface OptionDefinition<K extends keyof Options> {
  name: K;
  label: string;
  choices: OptionChoice<Options[K]>[];
}

const FOCUS_CLEAR: OptionDefinition<"focus_clear"> = {
  name: "focus_clear",
  label: "Focus Mode, when nothing needs you",
  choices: [
    { value: "all_clear", label: "All clear", hint: "A calm All clear message and nothing else." },
    {
      value: "activity_dashboard",
      label: "Activity Dashboard",
      hint: "How many Tickets are Ready and In Progress, and the In Progress Tickets.",
    },
  ],
};

// OPTIONS is every Option the view offers, in the order shown.
const OPTIONS = [FOCUS_CLEAR];

export interface OptionsViewProps {
  api: Api;
  // options is the live read of the Options: data once read, error if the
  // last read failed.
  options: { data?: Options; error?: ApiError };
  loc: UiLocation;
  navigate(patch: Partial<UiLocation>): void;
}

export function OptionsView({ api, options, loc, navigate }: OptionsViewProps) {
  // saved is what the last save answered, until the next read replaces it.
  const [saved, setSaved] = useState<Options>();
  const [error, setError] = useState<ApiError>();
  useEffect(() => setSaved(undefined), [options.data]);
  const shown = saved ?? options.data;
  const back = returnViewOf(loc);

  const save = (change: Partial<Options>) =>
    api.setOptions(change).then(
      (v) => {
        setSaved(v);
        setError(undefined);
      },
      (e: ApiError) => setError(e),
    );

  const shownError = error ?? (!shown ? options.error : undefined);
  return (
    <main aria-label="Options" className="options">
      <header className="options-head">
        <h1>Options</h1>
        <button className="btn options-back" onClick={() => navigate({ view: back })}>
          {back === "focus" ? "Back to Focus Mode" : "Back to the Inbox"}
        </button>
      </header>
      <div className="options-body">
        {shownError && (
          <p role="alert" className="warning">
            {shownError.message}
          </p>
        )}
        {OPTIONS.map((o) => (
          <Option key={o.name} option={o} value={shown?.[o.name]} disabled={!shown} onChange={(v) => save({ [o.name]: v })} />
        ))}
      </div>
    </main>
  );
}

interface OptionProps<K extends keyof Options> {
  option: OptionDefinition<K>;
  // value is the saved value; none until the Options are read.
  value?: Options[K];
  disabled: boolean;
  onChange(v: Options[K]): void;
}

// Option is one Option: its choices as radio buttons, the saved one checked.
function Option<K extends keyof Options>({ option, value, disabled, onChange }: OptionProps<K>) {
  const labelId = `option-${option.name}`;
  return (
    <section className="option">
      <h2 id={labelId}>{option.label}</h2>
      <div role="radiogroup" aria-labelledby={labelId} className="option-choices">
        {option.choices.map((c) => (
          <div key={c.value} className="option-choice">
            <label>
              <input
                type="radio"
                name={option.name}
                value={c.value}
                checked={value === c.value}
                disabled={disabled}
                aria-describedby={`${labelId}-${c.value}`}
                onChange={() => onChange(c.value)}
              />{" "}
              {c.label}
            </label>
            <p id={`${labelId}-${c.value}`} className="option-choice-hint muted">
              {c.hint}
            </p>
          </div>
        ))}
      </div>
    </section>
  );
}
