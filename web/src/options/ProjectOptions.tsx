// The Project Options page: one Project's settings, reached from its menu
// in the left pane, a section each: the Landing Method, CI with its CI
// Rules, and GitHub Issues.
import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import type {
  ApiError,
  CIProvider,
  CIRule,
  CIRuleChange,
  CISettings,
  CIWatcherStatus,
  GitHubIssuesChange,
  GitHubIssuesSettings,
  IssueWatcherStatus,
  Landing,
  LandingMethod,
  ProjectRow,
} from "../api/types";
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
          <GitHubIssuesSection api={api} project={project} />
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
    <fieldset className="project-options-section" disabled={project.archived}>
      <legend>CI</legend>
      <form onSubmit={save}>
        <div className="project-options-choice">
          <label>
            <input
              type="checkbox"
              checked={draft.tracking}
              onChange={(e) => edit({ tracking: e.target.checked })}
              aria-describedby={error?.field === "tracking" ? `${id}-tracking ${id}-error` : `${id}-tracking`}
              aria-invalid={error?.field === "tracking" || undefined}
            />{" "}
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
      </form>
      {project.ci.tracking && <WatcherStatus name="CI Watcher" repository={project.ci.repository} watcher={project.ci.watcher} />}
      <CIRules api={api} project={project} />
    </fieldset>
  );
}

// WatcherStatus is a Watcher's status, as it last reported it: Watching,
// with until when GitHub rate-limits it (code-central-260), or an error
// with its fix (code-central-253), or, as the service reports it, crashed
// and being restarted (code-central-255). It follows the Project as read,
// so it changes as the Watcher reports. The CI section shows the CI
// Watcher's, and the GitHub Issues section the Issue Watcher's
// (code-central-285).
function WatcherStatus({ name, repository, watcher }: { name: string; repository: string; watcher: CIWatcherStatus | IssueWatcherStatus }) {
  return (
    <section className="project-options-watcher" aria-label={name}>
      {watcher.state === "watching" && (
        <p>
          {name}: Watching {repository}.
        </p>
      )}
      {watcher.state === "watching" && watcher.rate_limited_until && (
        <p className="muted">
          GitHub is rate-limiting it until{" "}
          <time dateTime={watcher.rate_limited_until}>
            {new Date(watcher.rate_limited_until).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}
          </time>
          , so it waits until then, and carries on by itself.
        </p>
      )}
      {watcher.state === "not_reported" && <p className="muted">{name}: Starting; it hasn&apos;t reported yet.</p>}
      {watcher.state === "error" && (
        <p className="warning">
          {name}: Error. {watcher.message}
          {watcher.fix && (
            <>
              <br />
              {watcher.fix}
            </>
          )}
          {watcher.problem !== "crashed" && (
            <>
              <br />
              It tries again every 2 minutes, and goes back to Watching once the cause is fixed.
            </>
          )}
        </p>
      )}
    </section>
  );
}

// issuesDraft is the form for GitHub Issues as stored, its repository
// prefilled from a GitHub Push remote while none is set, as CI's is.
function issuesDraft(g: GitHubIssuesSettings, pushRemoteURL: string): GitHubIssuesChange {
  return { on: g.on, repository: g.repository || githubRepository(pushRemoteURL) || "", label: g.label, new_issues_need_me: g.new_issues_need_me };
}

// GitHubIssuesSection is the Project's GitHub Issues settings (Spec
// code-central-279): the switch, the GitHub repository its issues come
// from, the filter label and New issues need me. Turning it on runs the
// Issue Watcher's check, whose refusal shows in place with its fix; while
// it is on, the Issue Watcher's status shows below. Like the other
// sections it follows the Project as read.
function GitHubIssuesSection({ api, project }: { api: Api; project: ProjectRow }) {
  const stored = issuesDraft(project.github_issues, project.ci.push_remote_url);
  const prefilled = !project.github_issues.repository && stored.repository !== "";
  const [draft, setDraft] = useState<GitHubIssuesChange>(stored);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  const { on, repository, label, new_issues_need_me } = stored;
  useEffect(() => setDraft({ on, repository, label, new_issues_need_me }), [on, repository, label, new_issues_need_me]);

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
    draft.on !== on || draft.repository !== repository || draft.label.trim() !== label || draft.new_issues_need_me !== new_issues_need_me;
  const describedBy = (field: string, hint: string) => (error?.field === field ? `${hint} ${id}-error` : hint);
  const invalid = (field: string) => error?.field === field || undefined;
  const spec = project.github_issues.spec;

  return (
    <fieldset className="project-options-section" disabled={project.archived}>
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
            Each open issue in the repository arrives as an Incoming Issue, a Draft Ticket to triage, and Code Central writes the work&apos;s
            progress back to it, through your existing <code>gh</code> login. Code Central stores no GitHub token.
          </p>
        </div>
        <label className="project-options-field">
          GitHub repository{" "}
          <input
            value={draft.repository}
            placeholder="owner/name"
            onChange={(e) => edit({ repository: e.target.value })}
            aria-describedby={describedBy("repository", `${id}-repository`)}
            aria-invalid={invalid("repository")}
          />
        </label>
        <p className="project-options-hint" id={`${id}-repository`}>
          {prefilled ? `From the Push remote ${project.ci.push_remote_url}. ` : ""}The repository whose issues come in, as owner/name; GitHub
          Issues needs one.
        </p>
        <label className="project-options-field">
          Filter label{" "}
          <input
            value={draft.label}
            placeholder="any label"
            onChange={(e) => edit({ label: e.target.value })}
            aria-describedby={`${id}-label`}
          />
        </label>
        <p className="project-options-hint" id={`${id}-label`}>
          Only issues carrying this GitHub label come in. Leave it empty to bring in every open issue.
        </p>
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
            Each new Incoming Issue also shows in Needs Me and Focus Mode until you triage it.
          </p>
        </div>
        <p className="project-options-hint">For now the Issue Watcher only checks it can reach the repository: no issues come in yet.</p>
        {spec && <p className="muted">Incoming Issues arrive on {spec}.</p>}
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
      </form>
      {project.github_issues.on && (
        <WatcherStatus name="Issue Watcher" repository={project.github_issues.repository} watcher={project.github_issues.watcher} />
      )}
    </fieldset>
  );
}

// ruleWatches is what a CI Rule watches, as its Spec's title says it.
const ruleWatches = (r: CIRuleChange) => `${r.workflow === "any" ? "any workflow" : r.workflow} on ${r.branch}`;

const NO_RULE: CIRuleChange = { workflow: "", branch: "", state: "Draft" };

// CIRules, in the CI section, lists the Project's CI Rules ("when workflow
// X fails on branch Y, open a Ticket to fix it") and adds, edits and
// removes them. Each rule has its own Spec, which removing the rule keeps.
function CIRules({ api, project }: { api: Api; project: ProjectRow }) {
  const [editing, setEditing] = useState<string>();
  const id = useId();
  const { rules } = project.ci;

  return (
    <fieldset className="project-options-rules-section">
      <legend>CI Rules</legend>
      <p className="project-options-hint">
        A CI Rule says &ldquo;when this workflow fails on this branch, open a Ticket to fix it&rdquo;, on the rule&apos;s own Spec. For now
        rules are only recorded: none opens a Ticket yet.
      </p>
      {rules.length === 0 ? (
        <p className="muted">No CI Rules yet.</p>
      ) : (
        <ul className="project-options-rules" aria-label="CI Rules">
          {rules.map((r) =>
            editing === r.spec ? (
              <li key={r.spec}>
                <RuleForm
                  label={`Edit ${ruleWatches(r)}`}
                  initial={r}
                  submit="Save"
                  save={(change) => api.editCIRule(project.key, r.spec, change).then(() => setEditing(undefined))}
                  cancel={() => setEditing(undefined)}
                />
              </li>
            ) : (
              <RuleRow key={r.spec} rule={r} removeHint={`${id}-remove`} edit={() => setEditing(r.spec)} remove={() => api.removeCIRule(project.key, r.spec)} />
            ),
          )}
        </ul>
      )}
      <p hidden id={`${id}-remove`}>
        Removing a rule keeps its Spec, as the record of its Tickets.
      </p>
      <RuleForm label="Add a CI Rule" initial={NO_RULE} submit="Add rule" save={(change) => api.addCIRule(project.key, change)} clearOnSave />
    </fieldset>
  );
}

// RuleRow is one CI Rule, with its Edit and Remove buttons.
function RuleRow({ rule, removeHint, edit, remove }: { rule: CIRule; removeHint: string; edit(): void; remove(): Promise<unknown> }) {
  const [error, setError] = useState<ApiError>();
  const watches = ruleWatches(rule);
  return (
    <li>
      <span>
        <strong>{watches}</strong>: opens {rule.state} Tickets on {rule.spec}
      </span>{" "}
      <button type="button" className="btn" aria-label={`Edit ${watches}`} onClick={edit}>
        Edit
      </button>{" "}
      <button type="button" className="btn" aria-label={`Remove ${watches}`} aria-describedby={removeHint} onClick={() => remove().catch(setError)}>
        Remove
      </button>
      {error && (
        <p className="warning" role="alert">
          {error.message}
        </p>
      )}
    </li>
  );
}

// RuleForm is a CI Rule's fields: adding one, or editing one in place.
function RuleForm(props: {
  label: string;
  initial: CIRuleChange;
  submit: string;
  save(change: CIRuleChange): Promise<unknown>;
  cancel?(): void;
  clearOnSave?: boolean;
}) {
  const { label, initial, submit, save, cancel, clearOnSave } = props;
  const [draft, setDraft] = useState<CIRuleChange>({ workflow: initial.workflow, branch: initial.branch, state: initial.state });
  const [error, setError] = useState<ApiError>();
  const id = useId();

  const edit = (patch: Partial<CIRuleChange>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
  };
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    save(draft).then(
      () => clearOnSave && setDraft(NO_RULE),
      (e: ApiError) => setError(e),
    );
  };
  const invalid = (field: string) => (error?.field === field ? { "aria-invalid": true, "aria-describedby": `${id}-error` } : undefined);
  const filled = draft.workflow.trim() !== "" && draft.branch.trim() !== "";

  return (
    <form aria-label={label} className="project-options-rule" onSubmit={onSubmit}>
      <label className="project-options-field">
        Workflow{" "}
        <input
          value={draft.workflow}
          placeholder="ci.yml or any"
          onChange={(e) => edit({ workflow: e.target.value })}
          aria-describedby={`${id}-workflow`}
          {...invalid("workflow")}
        />
      </label>
      <p className="project-options-hint" id={`${id}-workflow`}>
        A workflow file name, such as ci.yml, or any for every workflow but the Ignored ones.
      </p>
      <label className="project-options-field">
        Branch{" "}
        <input
          value={draft.branch}
          placeholder="main"
          onChange={(e) => edit({ branch: e.target.value })}
          aria-describedby={`${id}-branch`}
          {...invalid("branch")}
        />
      </label>
      <p className="project-options-hint" id={`${id}-branch`}>
        An exact branch name, with no globs.
      </p>
      <label className="project-options-field">
        Tickets open as{" "}
        <select value={draft.state} onChange={(e) => edit({ state: e.target.value as CIRuleChange["state"] })} {...invalid("state")}>
          <option value="Draft">Draft</option>
          <option value="Ready">Ready</option>
        </select>
      </label>
      {error && (
        <p className="warning" role="alert" id={`${id}-error`}>
          {error.message}
        </p>
      )}
      <div className="project-options-actions">
        <button type="submit" className="btn primary" disabled={!filled}>
          {submit}
        </button>
        {cancel && (
          <button type="button" className="btn" onClick={cancel}>
            Cancel
          </button>
        )}
      </div>
    </form>
  );
}

// Refusal is a save the service refused: its message, then its fix when
// it gives one, as a refused CI Tracking or GitHub Issues turn-on does
// (code-central-252, code-central-284).
function Refusal({ id, error }: { id: string; error: ApiError }) {
  return (
    <p className="warning" role="alert" id={id}>
      {error.message}
      {error.fix && (
        <>
          <br />
          {error.fix}
        </>
      )}
    </p>
  );
}
