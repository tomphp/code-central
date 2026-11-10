// The Ticket detail's Model and effort (Spec code-central-301): the
// model and effort the Ticket's next turn runs with, each with where it
// comes from, and pickers that set or clear the Ticket's model and effort
// Tags.
import { useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import { EFFORT_LEVELS, type ChoiceValue, type ReportedModels, type TicketView } from "../api/types";
import { CLAUDE_CODE_DEFAULT } from "../format";
import { useAct } from "./useAct";

// NO_REPORT stands in for the Reported Models until the Project is read:
// the seeded levels, nothing to suggest and no Factory flags.
const NO_REPORT: ReportedModels = { effort_levels: EFFORT_LEVELS, aliases: [], model_ids: [], model_flag: "", effort_flag: "" };

// applied is what a value's line says: the value and its source. A value
// Code Central leaves unset falls to the Factory's flag, if it reported
// one, and else to Claude Code's default.
function applied(v: ChoiceValue, flag: string): string {
  switch (v.source) {
    case "ticket_tag":
      return `${v.value} (Ticket Tag)`;
    case "project_kind":
      return `${v.value} (the Project's ${v.kind} row)`;
    case "project_default":
      return `${v.value} (Project default)`;
    case "none":
      return flag ? `${flag} (Factory flag)` : CLAUDE_CODE_DEFAULT;
  }
}

export interface ModelChoiceProps {
  api: Api;
  project: string;
  ticket: TicketView;
  // reported is the Project's Reported Models, once read.
  reported?: ReportedModels;
}

// ModelChoice shows the Ticket's Model Choice, from the UI's Ticket read,
// with a picker for each Tag: the model is free text suggesting the
// Reported Models' aliases and model IDs, set with Set; the effort is one
// of the valid levels, set on picking. Clearing either removes its Tag,
// so the Ticket goes back to its Project's settings.
export function ModelChoice({ api, project, ticket: t, reported = NO_REPORT }: ModelChoiceProps) {
  const mc = t.model_choice;
  const id = useId();
  const { busy, error, run } = useAct();
  if (!mc) return null;
  const tag = { model: t.tags.model ?? "", effort: t.tags.effort ?? "" };
  const suggestions = [...new Set([...reported.aliases, ...reported.model_ids])];
  const valid = reported.effort_levels;
  const levels = tag.effort && !valid.includes(tag.effort) ? [...valid, tag.effort] : valid;
  const setEffort = (effort: string) =>
    run(() => (effort ? api.setTags(project, t.id, { effort }) : api.removeTags(project, t.id, ["effort"])));

  return (
    <fieldset className="model-choice small" aria-label="Model and effort">
      <div className="model-choice-row">
        <span data-testid="model-applies">Model: {applied(mc.model, reported.model_flag)}</span>
        {/* Keyed on the Tag, so the picker follows a change made elsewhere. */}
        <ModelPicker
          key={tag.model}
          tag={tag.model}
          list={suggestions.length ? `${id}-models` : undefined}
          busy={busy}
          set={(model) => run(() => api.setTags(project, t.id, { model }))}
          clear={() => run(() => api.removeTags(project, t.id, ["model"]))}
        />
        {suggestions.length > 0 && (
          <datalist id={`${id}-models`}>
            {suggestions.map((m) => (
              <option key={m} value={m} />
            ))}
          </datalist>
        )}
      </div>
      <div className="model-choice-row">
        <span data-testid="effort-applies">Effort: {applied(mc.effort, reported.effort_flag)}</span>
        <select aria-label="Effort Tag" value={tag.effort} disabled={busy} onChange={(e) => setEffort(e.target.value)}>
          <option value="">Project&apos;s settings</option>
          {levels.map((l) => (
            <option key={l} value={l}>
              {l}
            </option>
          ))}
        </select>
      </div>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
    </fieldset>
  );
}

function ModelPicker(props: { tag: string; list?: string; busy: boolean; set(model: string): void; clear(): void }) {
  const [draft, setDraft] = useState(props.tag);
  const model = draft.trim();
  const submit = (e: FormEvent) => {
    e.preventDefault();
    props.set(model);
  };
  return (
    <form onSubmit={submit}>
      <input aria-label="Model Tag" list={props.list} value={draft} placeholder="Project's settings" onChange={(e) => setDraft(e.target.value)} />{" "}
      <button type="submit" className="btn" aria-label="Set model" disabled={props.busy || !model || model === props.tag}>
        Set
      </button>
      {props.tag && (
        <>
          {" "}
          <button type="button" className="btn" aria-label="Clear model" disabled={props.busy} onClick={props.clear}>
            Clear
          </button>
        </>
      )}
    </form>
  );
}
