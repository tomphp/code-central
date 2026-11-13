// The Options view: the human's global preferences. Every Option chosen
// from a set is one entry in OPTIONS, saved the moment it's chosen, so a
// later one is another entry there; gh path, a path typed in, is saved
// with its own Save. Leaving returns to the view the human came from, as
// the URL's from says.
import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import type { ApiError, Options } from "../api/types";
import { returnViewOf, type UiLocation } from "../url";
import { Button } from "../controls/Button";
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
    {
      value: "activity_dashboard",
      label: "Activity Dashboard",
      hint: "How many Tickets are Ready and In Progress, and the In Progress Tickets.",
    },
    { value: "all_clear", label: "All clear", hint: "A calm All clear message and nothing else." },
  ],
};

const FOCUS_POPUP: OptionDefinition<"focus_popup"> = {
  name: "focus_popup",
  label: "Pop up Focus Items on other views",
  choices: [
    { value: "off", label: "Off", hint: "Needs Me waits for you in Focus Mode or a Board card." },
    {
      value: "on",
      label: "On",
      hint: "A popup shows each new Focus Item on every view but Focus Mode, Options and Project Options.",
    },
  ],
};

// OPTIONS is every Option chosen from a set, in the order shown.
const OPTIONS = [FOCUS_CLEAR, FOCUS_POPUP];

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

  // save saves change, and reports whether the service took it.
  const save = (change: Partial<Options>) =>
    api.setOptions(change).then(
      (v) => {
        setSaved(v);
        setError(undefined);
        return true;
      },
      (e: ApiError) => {
        setError(e);
        return false;
      },
    );

  const shownError = error ?? (!shown ? options.error : undefined);
  return (
    <main aria-label="Options" className="options">
      <header className="options-head">
        <h1>Options</h1>
        <Button className="options-back" onClick={() => navigate({ view: back })}>
          {back === "focus" ? "Back to Focus Mode" : "Back to the Inbox"}
        </Button>
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
        <GHPathOption value={shown?.gh_path} disabled={!shown} invalid={error?.field === "gh_path"} onSave={(gh_path) => save({ gh_path })} />
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

interface GHPathOptionProps {
  // value is the saved gh path; none until the Options are read.
  value?: string;
  disabled: boolean;
  // invalid marks the field when the service refused the last save.
  invalid: boolean;
  onSave(ghPath: string): Promise<boolean>;
}

// GHPathOption is the gh path Option (code-central-252): where gh, the
// GitHub CLI, is for CI Tracking, typed in and saved with Save.
function GHPathOption({ value = "", disabled, invalid, onSave }: GHPathOptionProps) {
  const [draft, setDraft] = useState(value);
  const [saved, setSaved] = useState(false);
  const id = useId();
  useEffect(() => setDraft(value), [value]);
  const changed = draft.trim() !== value;
  const save = (e: FormEvent) => {
    e.preventDefault();
    onSave(draft).then(setSaved);
  };
  return (
    <form onSubmit={save}>
      <fieldset className="option" disabled={disabled} aria-labelledby={`${id}-label`}>
        <h2 id={`${id}-label`}>gh path</h2>
        <input
          className="option-text"
          value={draft}
          placeholder="/opt/homebrew/bin/gh"
          aria-labelledby={`${id}-label`}
          aria-describedby={`${id}-hint`}
          aria-invalid={invalid || undefined}
          onChange={(e) => {
            setDraft(e.target.value);
            setSaved(false);
          }}
        />{" "}
        <Button type="submit"  disabled={!changed}>
          Save
        </Button>
        {saved && !changed && (
          <span role="status" className="muted">
            {" "}
            Saved
          </span>
        )}
        <p id={`${id}-hint`} className="option-hint muted">
          Where <code>gh</code>, the GitHub CLI that CI Tracking uses, is: the program or its folder. Leave it empty to find gh on the
          service&apos;s PATH, then in /opt/homebrew/bin, /usr/local/bin, /home/linuxbrew/.linuxbrew/bin or %ProgramFiles%\GitHub CLI.
        </p>
      </fieldset>
    </form>
  );
}
