import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import type {
  ApiError,
  Landing,
  LandingMethod,
  ProjectRow,
} from "../api/types";
import { Refusal } from "./Refusal";
import { WatcherStatus } from "./WatcherStatus";

const METHODS: { method: LandingMethod; label: string; hint: string }[] = [
  {
    method: "merge",
    label: "Merge",
    hint: "The Factory merges the accepted branch into the base and lands the Ticket.",
  },
  {
    method: "branch",
    label: "Leave as a branch",
    hint: "The Factory leaves the branch for you to merge, and lands the Ticket when it sees the merge in the base.",
  },
  {
    method: "pull_request",
    label: "Pull Request",
    hint: "The Factory pushes the accepted branch and a pull request is opened on your Code Host; the Ticket lands when you merge it there.",
  },
];

const CODE_HOST_NAMES = { github: "GitHub", azure_repos: "Azure Repos" };

// LandingMethodSection is how the Project's accepted work lands. It follows
// the Project as read, so a change from another tab shows here too.
export function LandingMethodSection({
  api,
  project,
}: {
  api: Api;
  project: ProjectRow;
}) {
  const { method, push, remote, land_per_spec, pat_env, base, code_host, repository } = project.landing;
  const [draft, setDraft] = useState<Landing>(project.landing);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  useEffect(
    () => setDraft((d) => ({ ...d, method, push, remote, land_per_spec, pat_env, base })),
    [method, push, remote, land_per_spec, pat_env, base],
  );

  const edit = (patch: Partial<Landing>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
    setSaved(false);
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    // The draft becomes what was stored: the service may tidy it into no change.
    const { method, push, remote, land_per_spec, pat_env, base } = draft;
    api.setLandingMethod(project.key, { method, push, remote, land_per_spec, pat_env, base }).then(
      (v) => {
        setDraft(v.landing);
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed =
    draft.method !== method ||
    draft.push !== push ||
    draft.remote !== remote ||
    draft.land_per_spec !== land_per_spec ||
    draft.pat_env !== pat_env ||
    draft.base !== base;
  const remoteError = error?.field === "remote";
  const baseError = error?.field === "base";
  const pullRequest = draft.method === "pull_request";
  const pushRemoteURL = project.ci.push_remote_url;

  return (
    <form onSubmit={save}>
      <fieldset id="project-options-landing-method" className="project-options-section" disabled={project.archived}>
        <legend>Landing Method</legend>
        {project.archived && (
          <p className="muted">
            The Project is archived: unarchive it to change its options.
          </p>
        )}
        <label className="project-options-remote">
          Base branch
          <input
            type="text"
            value={draft.base}
            onChange={(e) => edit({ base: e.target.value })}
            aria-invalid={baseError || undefined}
            aria-describedby={baseError ? `${id}-error` : undefined}
          />
        </label>
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
        {!pullRequest && (
          <div className="project-options-choice">
            <label>
              <input
                type="checkbox"
                checked={draft.push}
                onChange={(e) => edit({ push: e.target.checked })}
                aria-describedby={`${id}-push`}
              />{" "}
              Push
            </label>
            <p className="project-options-hint" id={`${id}-push`}>
              Merge pushes the base to the remote after merging; Leave as a branch
              pushes the Ticket's branch when you accept it. The Factory checks
              the remote exists, never force-pushes, and under Merge pushes the
              whole base, your own commits on it too.
            </p>
          </div>
        )}
        <div className="project-options-choice">
          <label>
            <input
              type="checkbox"
              checked={draft.land_per_spec}
              disabled={draft.method === "branch"}
              onChange={(e) => edit({ land_per_spec: e.target.checked })}
              aria-describedby={`${id}-land-per-spec`}
            />{" "}
            Land per Spec
          </label>
          <p className="project-options-hint" id={`${id}-land-per-spec`}>
            Under Merge, a Spec's work gathers on its own branch and lands in the
            base, with Push and CI Tracking, once every Ticket on it is accepted,
            instead of Ticket by Ticket. Under Pull Request the Spec gets one pull
            request. It has no effect under Leave as a branch.
          </p>
        </div>
        <label className="project-options-remote">
          Remote{" "}
          <input
            value={draft.remote}
            disabled={!draft.push && !pullRequest}
            onChange={(e) => edit({ remote: e.target.value })}
            aria-invalid={remoteError || undefined}
            aria-describedby={remoteError ? `${id}-error` : undefined}
          />
        </label>
        {pullRequest && (
          <>
            <p className="project-options-hint" role="status" aria-label="Code Host">
              {code_host
                ? `Code Host: ${CODE_HOST_NAMES[code_host]}, repository ${repository}.`
                : pushRemoteURL
                  ? `The Push remote ${pushRemoteURL} is on a host Pull Request doesn't support: it supports GitHub and Azure Repos.`
                  : "No Push remote URL is reported yet: start the Factory once so it reports it, and the Code Host is worked out from it."}
            </p>
            {code_host === "azure_repos" && (
              <label className="project-options-remote">
                PAT variable{" "}
                <input
                  value={draft.pat_env}
                  onChange={(e) => edit({ pat_env: e.target.value })}
                  aria-invalid={error?.field === "pat_env" || undefined}
                />
              </label>
            )}
          </>
        )}
        {error && <Refusal id={`${id}-error`} error={error} />}
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
        {project.landing.open_pull_requests.length > 0 && (
          <div role="status" aria-label="Open pull requests" className="muted">
            {project.landing.open_pull_requests.length === 1
              ? "1 pull request is still open on the Code Host."
              : `${project.landing.open_pull_requests.length} pull requests are still open on the Code Host.`}{" "}
            Close {project.landing.open_pull_requests.length === 1 ? "it" : "them"} there yourself:
            <ul>
              {project.landing.open_pull_requests.map((pr) => (
                <li key={pr.ticket}>
                  {pr.ticket}:{" "}
                  <a href={pr.link} target="_blank" rel="noreferrer">
                    {pr.link}
                  </a>
                </li>
              ))}
            </ul>
          </div>
        )}
        {project.landing.method === "pull_request" && (
          <WatcherStatus
            name="Pull Request Watcher"
            repository={project.landing.repository}
            watcher={project.landing.watcher}
          />
        )}
      </fieldset>
    </form>
  );
}
