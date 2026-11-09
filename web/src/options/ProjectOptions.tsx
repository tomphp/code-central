// The Project Options page: one Project's settings, reached from its menu
// in the left pane. Its one section today is the Landing Method; later
// settings are more sections here.
import { useEffect, useId, useState, type FormEvent, type MouseEvent } from "react";
import type { Api } from "../api/api";
import type { ApiError, Landing, LandingMethod, ProjectRow } from "../api/types";
import { hrefFor, opensInPlace, type UiLocation } from "../url";

export interface ProjectOptionsProps {
  api: Api;
  // projects is every Project, once read; the page shows the one keyed project.
  projects?: ProjectRow[];
  project?: string;
  navigate(patch: Partial<UiLocation>): void;
}

export function ProjectOptions({ api, projects, project: key, navigate }: ProjectOptionsProps) {
  const project = projects?.find((p) => p.key === key);
  const back: UiLocation = { view: "inbox", project: project?.key };
  const onBack = (e: MouseEvent) => {
    if (!opensInPlace(e)) return;
    e.preventDefault();
    navigate(back);
  };

  return (
    <main aria-label="Project Options" className="options">
      <a className="options-back" href={hrefFor(back)} onClick={onBack}>
        Back to the inbox
      </a>
      {!projects ? (
        <p className="empty">Loading…</p>
      ) : !project ? (
        <p className="empty">There is no Project {key}.</p>
      ) : (
        <>
          <h1>{project.display_name}: Project Options</h1>
          <LandingMethodSection api={api} project={project} />
        </>
      )}
    </main>
  );
}

const METHODS: { method: LandingMethod; label: string; does: string }[] = [
  {
    method: "merge",
    label: "Merge into base",
    does: "The Factory merges the accepted branch into the base and lands the Ticket.",
  },
  {
    method: "branch",
    label: "Leave as a branch",
    does: "The Factory leaves the branch for you to merge, and lands the Ticket when it sees the merge in the base.",
  },
];

// LandingMethodSection is how the Project's accepted work lands. It follows
// the Project as read, so a change from another tab shows here too.
function LandingMethodSection({ api, project }: { api: Api; project: ProjectRow }) {
  const { method, push, remote } = project.landing;
  const [draft, setDraft] = useState<Landing>(project.landing);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  useEffect(() => setDraft({ method, push, remote }), [method, push, remote]);

  const edit = (patch: Partial<Landing>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
    setSaved(false);
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    api.setLandingMethod(project.key, draft).then(
      () => setSaved(true),
      (e: ApiError) => setError(e),
    );
  };
  const changed = draft.method !== method || draft.push !== push || draft.remote !== remote;
  const remoteError = error?.field === "remote";

  return (
    <form onSubmit={save}>
      <fieldset className="options-section" disabled={project.archived}>
        <legend>Landing Method</legend>
        {project.archived && <p className="muted">The Project is archived: unarchive it to change its options.</p>}
        {METHODS.map((m) => (
          <div key={m.method} className="options-choice">
            <label>
              <input
                type="radio"
                name={`${id}-method`}
                checked={draft.method === m.method}
                onChange={() => edit({ method: m.method })}
                aria-describedby={`${id}-${m.method}`}
              />{" "}
              {m.label}
            </label>
            <p className="options-hint" id={`${id}-${m.method}`}>
              {m.does}
            </p>
          </div>
        ))}
        <div className="options-choice">
          <label>
            <input type="checkbox" checked={draft.push} onChange={(e) => edit({ push: e.target.checked })} aria-describedby={`${id}-push`} />{" "}
            Push
          </label>
          <p className="options-hint" id={`${id}-push`}>
            Merge pushes the base to the remote after merging; Leave as a branch pushes the Ticket's branch when you accept it. The Factory
            checks the remote exists, never force-pushes, and under Merge pushes the whole base, your own commits on it too.
          </p>
        </div>
        <label className="options-remote">
          Remote{" "}
          <input
            value={draft.remote}
            disabled={!draft.push}
            onChange={(e) => edit({ remote: e.target.value })}
            aria-invalid={remoteError || undefined}
            aria-describedby={remoteError ? `${id}-error` : undefined}
          />
        </label>
        {error && (
          <p className="warning" role="alert" id={`${id}-error`}>
            {error.message}
          </p>
        )}
        <div className="options-actions">
          <button type="submit" className="btn primary" disabled={!changed}>
            Save
          </button>
          {saved && !changed && (
            <span role="status" className="muted">
              Saved
            </span>
          )}
        </div>
      </fieldset>
    </form>
  );
}
