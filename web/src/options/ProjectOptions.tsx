// The Project Options page: one Project's settings, reached from its menu
// in the left pane, a section each: the Landing Method and CI.
import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import type { ApiError, CIProvider, CISettings, Landing, LandingMethod, ProjectRow } from "../api/types";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { githubRepository } from "./githubRepository";

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

  return (
    <main aria-label="Project Options" className="project-options">
      <a className="project-options-back" href={hrefFor(back)} onClick={clickTo(navigate, back)}>
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
          <CISection api={api} project={project} />
        </>
      )}
    </main>
  );
}

const METHODS: { method: LandingMethod; label: string; hint: string }[] = [
  {
    method: "merge",
    label: "Merge into base",
    hint: "The Factory merges the accepted branch into the base and lands the Ticket.",
  },
  {
    method: "branch",
    label: "Leave as a branch",
    hint: "The Factory leaves the branch for you to merge, and lands the Ticket when it sees the merge in the base.",
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
    // The draft becomes what was stored: the service may tidy it into no change.
    api.setLandingMethod(project.key, draft).then(
      (v) => {
        setDraft(v.landing);
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed = draft.method !== method || draft.push !== push || draft.remote !== remote;
  const remoteError = error?.field === "remote";

  return (
    <form onSubmit={save}>
      <fieldset className="project-options-section" disabled={project.archived}>
        <legend>Landing Method</legend>
        {project.archived && <p className="muted">The Project is archived: unarchive it to change its options.</p>}
        {METHODS.map((m) => (
          <div key={m.method} className="project-options-choice">
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
            <p className="project-options-hint" id={`${id}-${m.method}`}>
              {m.hint}
            </p>
          </div>
        ))}
        <div className="project-options-choice">
          <label>
            <input type="checkbox" checked={draft.push} onChange={(e) => edit({ push: e.target.checked })} aria-describedby={`${id}-push`} />{" "}
            Push
          </label>
          <p className="project-options-hint" id={`${id}-push`}>
            Merge pushes the base to the remote after merging; Leave as a branch pushes the Ticket's branch when you accept it. The Factory
            checks the remote exists, never force-pushes, and under Merge pushes the whole base, your own commits on it too.
          </p>
        </div>
        <label className="project-options-remote">
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
        <div className="project-options-actions">
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

const PROVIDERS: { provider: CIProvider; label: string }[] = [{ provider: "github_actions", label: "GitHub Actions" }];

// CIDraft is the CI section's form: Ignored workflows as typed, one string.
interface CIDraft {
  tracking: boolean;
  provider: CIProvider;
  repository: string;
  ignored: string;
}

// ciDraft is the form for ci as stored, its repository prefilled from a
// GitHub Push remote while none is set.
function ciDraft(ci: CISettings): CIDraft {
  return {
    tracking: ci.tracking,
    provider: ci.provider,
    repository: ci.repository || githubRepository(ci.push_remote_url) || "",
    ignored: ci.ignored_workflows.join(", "),
  };
}

// workflowNames splits Ignored workflows as typed, by commas or spaces.
const workflowNames = (ignored: string) => ignored.split(/[\s,]+/).filter(Boolean);

// CISection is the Project's CI settings: CI Tracking, its CI Provider,
// the GitHub repository it watches and the Ignored workflows. Like the
// Landing Method it follows the Project as read.
function CISection({ api, project }: { api: Api; project: ProjectRow }) {
  const stored = ciDraft(project.ci);
  const prefilled = !project.ci.repository && stored.repository !== "";
  const [draft, setDraft] = useState<CIDraft>(stored);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  const { tracking, provider, repository, ignored } = stored;
  useEffect(() => setDraft({ tracking, provider, repository, ignored }), [tracking, provider, repository, ignored]);

  const edit = (patch: Partial<CIDraft>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
    setSaved(false);
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    const change = { tracking: draft.tracking, provider: draft.provider, repository: draft.repository, ignored_workflows: workflowNames(draft.ignored) };
    api.setCITracking(project.key, change).then(
      (v) => {
        setDraft(ciDraft(v.ci));
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed =
    draft.tracking !== tracking ||
    draft.provider !== provider ||
    draft.repository !== repository ||
    workflowNames(draft.ignored).join(",") !== workflowNames(ignored).join(",");
  const invalid = (field: string) => (error?.field === field ? { "aria-invalid": true, "aria-describedby": `${id}-error` } : undefined);

  return (
    <form onSubmit={save}>
      <fieldset className="project-options-section" disabled={project.archived}>
        <legend>CI</legend>
        <div className="project-options-choice">
          <label>
            <input type="checkbox" checked={draft.tracking} onChange={(e) => edit({ tracking: e.target.checked })} aria-describedby={`${id}-tracking`} />{" "}
            CI Tracking
          </label>
          <p className="project-options-hint" id={`${id}-tracking`}>
            CI Tracking watches the CI of what the Factory pushes, through your existing <code>gh</code> login. Code Central stores no
            GitHub token.
          </p>
        </div>
        <label className="project-options-field">
          CI Provider{" "}
          <select value={draft.provider} onChange={(e) => edit({ provider: e.target.value as CIProvider })}>
            {PROVIDERS.map((p) => (
              <option key={p.provider} value={p.provider}>
                {p.label}
              </option>
            ))}
          </select>
        </label>
        <label className="project-options-field">
          GitHub repository{" "}
          <input
            value={draft.repository}
            placeholder="owner/name"
            onChange={(e) => edit({ repository: e.target.value })}
            aria-describedby={`${id}-repository`}
            {...invalid("repository")}
          />
        </label>
        <p className="project-options-hint" id={`${id}-repository`}>
          {prefilled ? `From the Push remote ${project.ci.push_remote_url}. ` : ""}The repository to watch, as owner/name; CI Tracking needs one.
        </p>
        <label className="project-options-field">
          Ignored workflows{" "}
          <input
            value={draft.ignored}
            placeholder="nightly.yml, preview.yml"
            onChange={(e) => edit({ ignored: e.target.value })}
            aria-describedby={`${id}-ignored`}
            {...invalid("ignored_workflows")}
          />
        </label>
        <p className="project-options-hint" id={`${id}-ignored`}>
          Workflow file names, such as nightly.yml, separated by commas: their runs never count towards a Ticket&apos;s CI result.
        </p>
        {error && (
          <p className="warning" role="alert" id={`${id}-error`}>
            {error.message}
          </p>
        )}
        <div className="project-options-actions">
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
