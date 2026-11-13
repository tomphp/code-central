import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import { AUTO_ACCEPT_KINDS, type ApiError, type AutoAccept, type AutoAcceptKind, type ProjectRow } from "../api/types";
import { Button } from "../controls/Button";
import { AUTO_ACCEPT_KIND_NAME } from "../ticket/autoAccept";
import { Refusal } from "./Refusal";

// AutoAcceptSection is the Project's Auto-accept setting (Spec
// code-central-536): one checkbox per Auto-accept kind and one Save. Like
// the other sections it follows the Project as read.
export function AutoAcceptSection({ api, project }: { api: Api; project: ProjectRow }) {
  const stored = project.auto_accept;
  const [draft, setDraft] = useState<AutoAccept>(stored);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  useEffect(() => setDraft(stored), [stored]);

  const changed = AUTO_ACCEPT_KINDS.filter((k) => draft[k] !== stored[k]);
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    const kinds: Partial<AutoAccept> = {};
    for (const k of changed) kinds[k] = draft[k];
    api.setAutoAccept(project.key, kinds).then(
      (v) => {
        setDraft(v.auto_accept);
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const toggle = (kind: AutoAcceptKind, on: boolean) => {
    setDraft({ ...draft, [kind]: on });
    setError(undefined);
    setSaved(false);
  };

  return (
    <fieldset id="project-options-auto-accept" className="project-options-section" disabled={project.archived}>
      <legend>Auto-accept</legend>
      <form onSubmit={save}>
        <p id={`${id}-hint`} className="project-options-hint">
          While a kind is on, a Ticket of that kind that finishes is accepted for you at once, so it never waits in
          In Review. Turning a kind on accepts nothing already In Review. Prototypes and Map Tickets are never
          auto-accepted.
        </p>
        {AUTO_ACCEPT_KINDS.map((kind) => (
          <div key={kind}>
            <label>
              <input
                type="checkbox"
                checked={draft[kind]}
                onChange={(e) => toggle(kind, e.target.checked)}
                aria-describedby={`${id}-hint`}
              />{" "}
              {AUTO_ACCEPT_KIND_NAME[kind]}
            </label>
          </div>
        ))}
        {error && <Refusal id={`${id}-error`} error={error} />}
        <div className="project-options-actions">
          <Button type="submit" variant="primary" disabled={!changed.length}>
            Save
          </Button>
          {saved && !changed.length && (
            <span role="status" className="muted">
              Saved
            </span>
          )}
        </div>
      </form>
    </fieldset>
  );
}
