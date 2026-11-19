import { useEffect, useId, useState, type FormEvent, type KeyboardEvent } from "react";
import { useApi } from "../live";
import type {
  ApiError,
  GitHubIssuesChange,
  GitHubIssuesSettings,
  IssueWatcherStatus,
  ProjectRow,
} from "../api/types";
import { Button } from "../controls/Button";
import { ErrorBanner } from "../controls/ErrorBanner";
import { Field } from "../controls/Field";
import { githubRepository } from "./githubRepository";
import { Refusal } from "./Refusal";
import { WatcherStatus } from "./WatcherStatus";

// issuesDraft is the form for GitHub Issues as stored, its repositories
// prefilled from a GitHub Push remote while none is set, as CI's is.
function issuesDraft(
  g: GitHubIssuesSettings,
  pushRemoteURL: string,
): GitHubIssuesChange {
  const prefill = githubRepository(pushRemoteURL);
  return {
    on: g.on,
    repositories: g.repositories.length > 0 ? g.repositories : prefill ? [prefill] : [],
    label: g.label,
    new_issues_need_me: g.new_issues_need_me,
  };
}

const sameList = (a: string[], b: string[]) =>
  a.length === b.length && a.every((r, i) => r === b[i]);

// listed reports whether repository is among repositories, as GitHub
// compares names: whatever their case.
const listed = (repositories: string[], repository: string) =>
  repositories.some((r) => r.toLowerCase() === repository.toLowerCase());

// repositoryStatus is what the Issue Watcher last said of repository: its
// own problem and fix while it is among those it can't watch, or Watching
// while the others are the ones in error.
function repositoryStatus(
  watcher: IssueWatcherStatus,
  repository: string,
): { text: string; fix?: string; error: boolean } {
  if (watcher.state === "not_reported") {
    return { text: "Starting; it hasn't reported yet.", error: false };
  }
  if (watcher.state === "elsewhere") return { text: `Runs on ${watcher.host ?? ""}`, error: false };
  if (watcher.state === "waiting") return { text: watcher.message ?? "", error: false };
  if (watcher.state === "watching") return { text: "Watching", error: false };
  const failing = watcher.repositories ?? [];
  if (failing.length === 0) {
    return { text: `Error. ${watcher.message ?? ""}`, fix: watcher.fix, error: true };
  }
  const own = failing.find((r) => r.repository === repository);
  return own
    ? { text: `Error. ${own.message}`, fix: own.fix, error: true }
    : { text: "Watching", error: false };
}

// GitHubIssuesSection is the Project's GitHub Issues settings (Spec
// code-central-279, code-central-848): the switch, the list of GitHub
// repositories its issues come from, the filter label and New issues need
// me, the last two for every repository. Each Repository's GitHub remote
// is offered as a suggestion. Turning it on, or adding a repository, runs
// the Issue Watcher's check, whose refusal shows in place with its fix;
// while it is on, the Issue Watcher's status shows below, with one row
// for each of several repositories. Like the other sections it follows the
// Project as read.
export function GitHubIssuesSection({ project }: { project: ProjectRow }) {
  const api = useApi();
  const stored = issuesDraft(project.github_issues, project.ci.push_remote_url);
  const prefilled =
    project.github_issues.repositories.length === 0 &&
    stored.repositories.length > 0;
  const [draft, setDraft] = useState<GitHubIssuesChange>(stored);
  const [adding, setAdding] = useState("");
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  const { on, repositories, label, new_issues_need_me } = stored;
  // The list as one string, so the effect below re-runs only when the
  // stored list changes, not on every read's new array.
  const storedRepositories = repositories.join("\n");
  useEffect(
    () => setDraft({ on, repositories: storedRepositories ? storedRepositories.split("\n") : [], label, new_issues_need_me }),
    [on, storedRepositories, label, new_issues_need_me],
  );

  // pending is the repository typed but not yet added: saving adds it too.
  const pending = adding.trim();
  const wanted =
    pending && !listed(draft.repositories, pending)
      ? [...draft.repositories, pending]
      : draft.repositories;
  const edit = (patch: Partial<GitHubIssuesChange>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
    setSaved(false);
  };
  const add = (repository: string) => {
    if (repository && !listed(draft.repositories, repository)) {
      edit({ repositories: [...draft.repositories, repository] });
    }
    setAdding("");
  };
  const onAddKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      add(pending);
    }
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    api.setGitHubIssues(project.key, { ...draft, repositories: wanted }).then(
      (v) => {
        setDraft(issuesDraft(v.github_issues, v.ci.push_remote_url));
        setAdding("");
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed =
    draft.on !== on ||
    !sameList(wanted, repositories) ||
    draft.label.trim() !== label ||
    draft.new_issues_need_me !== new_issues_need_me;
  const describedBy = (field: string, hint: string) =>
    error?.field === field ? `${hint} ${id}-error` : hint;
  const invalid = (field: string) => error?.field === field || undefined;
  const spec = project.github_issues.spec;
  const several = repositories.length > 1;
  // suggestions are each Repository's GitHub remote that isn't listed yet.
  const suggestions = project.repositories.flatMap((r) => {
    const repository = githubRepository(r.clone_url);
    return repository && !listed(wanted, repository) ? [{ repository, name: r.name }] : [];
  });
  const suggested = suggestions.filter(
    (s, i) => suggestions.findIndex((o) => o.repository === s.repository) === i,
  );

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
            Each open issue in the repositories arrives as an Incoming Issue, a
            Draft Ticket to triage, and Code Central writes the work&apos;s
            progress back to it, through your existing <code>gh</code> login.
            Code Central stores no GitHub token.
          </p>
        </div>
        <ul className="project-options-repositories" aria-label="GitHub repositories">
          {draft.repositories.map((repository) => {
            const status =
              several && on && listed(repositories, repository)
                ? repositoryStatus(project.github_issues.watcher, repository)
                : undefined;
            return (
              <li key={repository} aria-label={repository}>
                <span className="repo-name">{repository}</span>
                {status &&
                  (status.error ? (
                    <ErrorBanner>
                      {status.text}
                      {status.fix && (
                        <>
                          <br />
                          {status.fix}
                        </>
                      )}
                    </ErrorBanner>
                  ) : (
                    <span className="muted">
                      {": "}
                      {status.text}
                    </span>
                  ))}{" "}
                <Button
                  type="button"
                  aria-label={`Remove ${repository}`}
                  onClick={() =>
                    edit({ repositories: draft.repositories.filter((r) => r !== repository) })
                  }
                >
                  Remove
                </Button>
              </li>
            );
          })}
        </ul>
        <div className="project-options-add">
          <Field label="GitHub repository">
            <input
              value={adding}
              placeholder="owner/name"
              onChange={(e) => {
                setAdding(e.target.value);
                setError(undefined);
                setSaved(false);
              }}
              onKeyDown={onAddKey}
              aria-describedby={describedBy("repositories", `${id}-repository`)}
              aria-invalid={invalid("repositories")}
            />
          </Field>
          <Button type="button" disabled={pending === ""} onClick={() => add(pending)}>
            Add repository
          </Button>
        </div>
        <p className="field-hint" id={`${id}-repository`}>
          {prefilled
            ? `From the Push remote ${project.ci.push_remote_url}. `
            : ""}
          A repository whose issues come in, as owner/name; GitHub Issues
          needs one. Add as many as you like: the filter label and New
          issues need me apply to every one.
        </p>
        {suggested.length > 0 && (
          <ul className="project-options-repositories" aria-label="Suggested repositories">
            {suggested.map(({ repository, name }) => (
              <li key={repository}>
                <Button type="button" onClick={() => add(repository)}>
                  {`Add ${repository}`}
                </Button>
                {project.repositories.length > 1 && (
                  <span className="muted"> The remote of {name}.</span>
                )}
              </li>
            ))}
          </ul>
        )}
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
          repository={project.github_issues.repositories.join(", ")}
          watcher={project.github_issues.watcher}
          inRows={several}
        />
      )}
    </fieldset>
  );
}
