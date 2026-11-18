import { useEffect, useId, useState, type FormEvent } from "react";
import { useApi } from "../live";
import type { ApiError, AutoFixView, ProjectRow } from "../api/types";
import { Button } from "../controls/Button";
import { Field } from "../controls/Field";
import { Refusal } from "./Refusal";

// AutoFixDraft is the Auto-fix block's form: the limit as typed.
interface AutoFixDraft {
  on: boolean;
  instructions: string;
  limit: string;
}

const draftOf = (a: AutoFixView): AutoFixDraft => ({
  on: a.on,
  instructions: a.fix_instructions,
  limit: String(a.limit),
});

// AutoFixBlock is the Auto-fix setting (Spec code-central-1022) in the CI
// section of Project Options: the switch, the Fix instructions with a way
// back to the default, and the limit, saved with set_auto_fix. The caller
// shows it only while CI Tracking is on; Go checks every value.
export function AutoFixBlock({ project }: { project: ProjectRow }) {
  const api = useApi();
  const stored = project.ci.auto_fix;
  const [draft, setDraft] = useState<AutoFixDraft>(draftOf(stored));
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  const { on, fix_instructions, limit } = stored;
  useEffect(() => setDraft({ on, instructions: fix_instructions, limit: String(limit) }), [on, fix_instructions, limit]);

  const edit = (patch: Partial<AutoFixDraft>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
    setSaved(false);
  };
  const changed = draft.on !== on || draft.instructions !== fix_instructions || draft.limit !== String(limit);
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    api
      .setAutoFix(project.key, { on: draft.on, fix_instructions: draft.instructions, limit: Number(draft.limit) })
      .then(
        (v) => {
          setDraft(draftOf(v.ci.auto_fix));
          setSaved(true);
        },
        (e: ApiError) => setError(e),
      );
  };
  const invalid = (field: string) =>
    error?.field === field ? { "aria-invalid": true, "aria-describedby": `${id}-error` } : undefined;

  return (
    <form onSubmit={save} role="group" aria-label="Auto-fix">
      <div className="project-options-choice">
        <label>
          <input
            type="checkbox"
            checked={draft.on}
            onChange={(e) => edit({ on: e.target.checked })}
            aria-describedby={`${id}-on`}
            {...invalid("on")}
          />{" "}
          Auto-fix
        </label>
        <p className="project-options-hint" id={`${id}-on`}>
          While on, a CI failure opens a fix Ticket for you instead of waiting in Needs Me, up to the limit of
          attempts for one failure. Past it, the failure comes back to you.
        </p>
      </div>
      <Field
        label="Fix instructions"
        hintId={`${id}-instructions`}
        hint="Added to every fix Ticket's description after its run summary, whether Auto-fix or you open it."
      >
        <textarea
          rows={6}
          value={draft.instructions}
          onChange={(e) => edit({ instructions: e.target.value })}
          aria-describedby={`${id}-instructions`}
          {...invalid("fix_instructions")}
        />
      </Field>
      <div className="project-options-actions">
        <Button
          type="button"
          disabled={draft.instructions === stored.default_fix_instructions}
          onClick={() => edit({ instructions: stored.default_fix_instructions })}
        >
          Use the default instructions
        </Button>
      </div>
      <Field label="Auto-fix limit" hintId={`${id}-limit`} hint="The most attempts per failure, 1 to 10.">
        <input
          type="number"
          value={draft.limit}
          onChange={(e) => edit({ limit: e.target.value })}
          aria-describedby={`${id}-limit`}
          {...invalid("limit")}
        />
      </Field>
      {error && <Refusal id={`${id}-error`} error={error} />}
      <div className="project-options-actions">
        <Button type="submit" variant="primary" disabled={!changed}>
          Save Auto-fix
        </Button>
        {saved && !changed && (
          <span role="status" className="muted">
            Saved
          </span>
        )}
      </div>
    </form>
  );
}
