import { useEffect, useId, useState, type FormEvent } from "react";
import { useApi } from "../live";
import type { ApiError, ProjectRow } from "../api/types";
import { Button } from "../controls/Button";
import { Field } from "../controls/Field";
import { Refusal } from "./Refusal";

// CheckCommandForm is the Project's Check Command, the default for each
// Repository without its own, with its own Save.
export function CheckCommandForm({ project }: { project: ProjectRow }) {
  const api = useApi();
  const stored = project.check_command;
  const [draft, setDraft] = useState(stored);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  useEffect(() => setDraft(stored), [stored]);

  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    api.setCheckCommand(project.key, draft).then(
      (p) => {
        setDraft(p.check_command);
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed = draft !== stored;

  return (
    <form aria-label="Check Command" onSubmit={save}>
      <Field
        label="Check Command"
        hintId={`${id}-hint`}
        hint={
          <>
            The command an agent runs in a Repository to check its code, such as <code>npm run check</code>. Today
            only Conflict Claims run it, after resolving a merge or rebase. Left empty, they follow the
            Repository&apos;s own docs, such as CLAUDE.md or README, instead.
          </>
        }
      >
        <input
          value={draft}
          placeholder="npm run check"
          onChange={(e) => {
            setDraft(e.target.value);
            setError(undefined);
            setSaved(false);
          }}
          aria-describedby={`${id}-hint`}
          {...(error?.field === "check_command" ? { "aria-invalid": true } : undefined)}
        />
      </Field>
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
  );
}
