import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import type {
  ApiError,
  GitHubIssuesChange,
  GitHubIssuesSettings,
  ProjectRow,
} from "../api/types";
import { Button } from "../controls/Button";
import { Field } from "../controls/Field";
import { githubRepository } from "./githubRepository";
import { Refusal } from "./Refusal";
import { WatcherStatus } from "./WatcherStatus";

// issuesDraft is the form for GitHub Issues as stored, its repository
// prefilled from a GitHub Push remote while none is set, as CI's is.
function issuesDraft(
  g: GitHubIssuesSettings,
  pushRemoteURL: string,
): GitHubIssuesChange {
  return {
    on: g.on,
    repository: g.repository || githubRepository(pushRemoteURL) || "",
    label: g.label,
    new_issues_need_me: g.new_issues_need_me,
  };
}

// GitHubIssuesSection is the Project's GitHub Issues settings (Spec
// code-central-279): the switch, the GitHub repository its issues come
// from, the filter label and New issues need me. Turning it on runs the
// Issue Watcher's check, whose refusal shows in place with its fix; while
// it is on, the Issue Watcher's status shows below. Like the other
// sections it follows the Project as read.
export function GitHubIssuesSection({
  api,
  project,
}: {
  api: Api;
  project: ProjectRow;
}) {
  const stored = issuesDraft(project.github_issues, project.ci.push_remote_url);
  const prefilled =
    !project.github_issues.repository && stored.repository !== "";
  const [draft, setDraft] = useState<GitHubIssuesChange>(stored);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  const { on, repository, label, new_issues_need_me } = stored;
  useEffect(
    () => setDraft({ on, repository, label, new_issues_need_me }),
    [on, repository, label, new_issues_need_me],
  );

  const edit = (patch: Partial<GitHubIssuesChange>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
    setSaved(false);
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    api.setGitHubIssues(project.key, draft).then(
      (v) => {
        setDraft(issuesDraft(v.github_issues, v.ci.push_remote_url));
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed =
    draft.on !== on ||
    draft.repository !== repository ||
    draft.label.trim() !== label ||
    draft.new_issues_need_me !== new_issues_need_me;
  const describedBy = (field: string, hint: string) =>
    error?.field === field ? `${hint} ${id}-error` : hint;
  const invalid = (field: string) => error?.field === field || undefined;
  const spec = project.github_issues.spec;

  return (
    <fieldset id="project-options-github-issues" className="project-options-section" disabled={project.archived}>
      <legend>GitHub Issues</legend>
      <form onSubmit={save}>
        <div className="project-options-choice">
          <label>
            <input
              type="checkbox"
              checked={draft.on}
              onChange={(e) => edit({ on: e.target.checked })}
              aria-describedby={describedBy("on", `${id}-on`)}
              aria-invalid={invalid("on")}
            />{" "}
            GitHub Issues
          </label>
          <p className="project-options-hint" id={`${id}-on`}>
            Each open issue in the repository arrives as an Incoming Issue, a
            Draft Ticket to triage, and Code Central writes the work&apos;s
            progress back to it, through your existing <code>gh</code> login.
            Code Central stores no GitHub token.
          </p>
        </div>
        <Field
          label="GitHub repository"
          hintId={`${id}-repository`}
          hint={
            <>
          {prefilled
            ? `From the Push remote ${project.ci.push_remote_url}. `
            : ""}
          The repository whose issues come in, as owner/name; GitHub Issues
          needs one.
            </>
          }
        >
          <input
            value={draft.repository}
            placeholder="owner/name"
            onChange={(e) => edit({ repository: e.target.value })}
            aria-describedby={describedBy("repository", `${id}-repository`)}
            aria-invalid={invalid("repository")}
          />
        </Field>
        <Field
          label="Filter label"
          hintId={`${id}-label`}
          hint={
            <>
          Only issues carrying this GitHub label come in. Leave it empty to
          bring in every open issue.
            </>
          }
        >
          <input
            value={draft.label}
            placeholder="any label"
            onChange={(e) => edit({ label: e.target.value })}
            aria-describedby={`${id}-label`}
          />
        </Field>
        <div className="project-options-choice">
          <label>
            <input
              type="checkbox"
              checked={draft.new_issues_need_me}
              onChange={(e) => edit({ new_issues_need_me: e.target.checked })}
              aria-describedby={`${id}-need-me`}
            />{" "}
            New issues need me
          </label>
          <p className="project-options-hint" id={`${id}-need-me`}>
            Each new Incoming Issue also shows in Needs Me and Focus Mode until
            you triage it.
          </p>
        </div>
        <p className="project-options-hint">
          For now the Issue Watcher only checks it can reach the repository: no
          issues come in yet.
        </p>
        {spec && <p className="muted">Incoming Issues arrive on {spec}.</p>}
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
      {project.github_issues.on && (
        <WatcherStatus
          name="Issue Watcher"
          repository={project.github_issues.repository}
          watcher={project.github_issues.watcher}
        />
      )}
    </fieldset>
  );
}
