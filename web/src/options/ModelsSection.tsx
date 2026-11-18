import { useEffect, useId, useState, type FormEvent } from "react";
import { useApi } from "../live";
import type { ApiError, ModelRow, Models, ProjectRow } from "../api/types";
import { Select } from "../controls/Select";
import { CLAUDE_CODE_DEFAULT } from "../format";
import {
  CHAT_DEFAULT_MODEL,
  effortOptions,
  modelSuggestions,
  ModelSuggestions,
} from "../modelPickers";
import { Button } from "../controls/Button";
import { Refusal } from "./Refusal";

// ModelsSection is the Project's Models (Spec code-central-301): the
// model and effort for the Project default, then for each kind of work,
// then for Chat (Spec code-central-633), which the default doesn't reach.
// Each row's model is free text, empty for Default, suggesting the
// aliases and model IDs the Factory reported, and its effort one of the
// valid levels or Default; Default shows what it inherits. Save replaces
// the rows whole. Like the other sections it follows the Project as read.
export function ModelsSection({ project }: { project: ProjectRow }) {
  const api = useApi();
  const stored = JSON.stringify(project.models);
  const [draft, setDraft] = useState<Models>(project.models);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  useEffect(() => setDraft(JSON.parse(stored) as Models), [stored]);

  // edit patches the row whose field prefix is given: "default", "chat",
  // or "kinds.<kind>".
  const edit = (field: string, patch: Partial<ModelRow>) => {
    setDraft((d) =>
      // landing: a Models row's field prefix, not a Landing
      field === "default" || field === "chat"
        ? { ...d, [field]: { ...d[field], ...patch } }
        : {
            ...d,
            kinds: d.kinds.map((k) =>
              `kinds.${k.kind}` === field ? { ...k, ...patch } : k,
            ),
          },
    );
    setError(undefined);
    setSaved(false);
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    api.setModels(project.key, draft).then(
      (v) => {
        setDraft(v.models);
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed = JSON.stringify(draft) !== stored;
  const reported = project.reported_models;
  const suggestions = modelSuggestions(reported);
  // A kind's row inherits the Project default as typed; the Project
  // default inherits the Factory's flag, if it reported one, and else
  // Claude Code's default.
  const flags: ModelRow = {
    model: reported.model_flag,
    effort: reported.effort_flag,
  };
  const inherited = (kind: string | undefined, value: keyof ModelRow) => {
    const def = kind !== undefined && draft.default[value].trim();
    return `Default (${def || (flags[value] ? `Factory flag: ${flags[value]}` : CLAUDE_CODE_DEFAULT)})`;
  };
  // Each row has its kind (none for the Project default and Chat), its
  // name in the pickers' labels, the prefix of its fields in a refusal and
  // what its pickers show when empty. Chat doesn't inherit the Project
  // default or the Factory's flags: it falls to haiku and no effort.
  const rows: {
    kind?: string;
    name: string;
    header: string;
    field: string;
    row: ModelRow;
    placeholders: ModelRow;
  }[] = [
    {
      name: "the Project default",
      header: "Project default",
      field: "default",
      row: draft.default,
      placeholders: { model: inherited(undefined, "model"), effort: inherited(undefined, "effort") },
    },
    ...draft.kinds.map((k) => ({
      kind: k.kind,
      name: k.kind,
      header: k.kind,
      field: `kinds.${k.kind}`,
      row: k,
      placeholders: { model: inherited(k.kind, "model"), effort: inherited(k.kind, "effort") },
    })),
    {
      name: "Chat",
      header: "Chat",
      field: "chat",
      row: draft.chat,
      placeholders: { model: CHAT_DEFAULT_MODEL, effort: `Default (${CLAUDE_CODE_DEFAULT})` },
    },
  ];
  const invalid = (field: string) =>
    error?.field === field
      ? { "aria-invalid": true, "aria-describedby": `${id}-error` }
      : undefined;

  return (
    <fieldset id="project-options-models" className="project-options-section" disabled={project.archived}>
      <legend>Models</legend>
      <p className="project-options-hint project-options-models-hint">
        The model and effort the Factory&apos;s sessions run with, for each kind
        of work. A Ticket&apos;s model and effort Tags come first, then its
        kind&apos;s row here, then the Project default; what is left at Default
        falls to the Factory&apos;s --model and --effort, and then to Claude
        Code&apos;s default. A change applies from each Ticket&apos;s next turn.
        Chat has a row of its own: neither the Project default nor the
        Factory&apos;s flags reach it, and an empty model there means haiku.
      </p>
      <form onSubmit={save}>
        <table className="project-options-models">
          <thead>
            <tr>
              <th scope="col">Kind of work</th>
              <th scope="col">Model</th>
              <th scope="col">Effort</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(({ name, header, field, row, placeholders }) => {
              const levels = effortOptions(reported.effort_levels, row.effort);
              return (
                <tr key={header}>
                  <th scope="row">{header}</th>
                  <td>
                    <input
                      list={suggestions.length ? `${id}-models` : undefined}
                      aria-label={`Model for ${name}`}
                      value={row.model}
                      placeholder={placeholders.model}
                      onChange={(e) => edit(field, { model: e.target.value })}
                      {...invalid(`${field}.model`)}
                    />
                  </td>
                  <td>
                    <Select
                      aria-label={`Effort for ${name}`}
                      value={row.effort}
                      onChange={(v) => edit(field, { effort: v })}
                      placeholder={placeholders.effort}
                      options={levels.map((l) => ({ value: l }))}
                      {...invalid(`${field}.effort`)}
                    />
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
        <ModelSuggestions id={`${id}-models`} suggestions={suggestions} />
        {error && <Refusal id={`${id}-error`} error={error} />}
        <div className="project-options-actions">
          <Button type="submit" variant="primary" disabled={!changed}>
            Save
          </Button>
          {saved && !changed && (
            <span role="status" className="muted">
              Saved
            </span>
          )}
        </div>
      </form>
    </fieldset>
  );
}
