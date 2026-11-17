import { useId, useState, type FormEvent, type MouseEvent } from "react";
import { useApi } from "../live";
import type {
  ApiError,
  Landing,
  ProjectRow,
  RepositoryAdd,
  RepositoryLanding,
  RepositoryView,
  CloneView,
  CodeHost,
} from "../api/types";
import { Button } from "../controls/Button";
import { Field } from "../controls/Field";
import { Select } from "../controls/Select";
import { Refusal } from "./Refusal";
import { RepositoryCI } from "./RepositoryCI";

// LANDINGS name each Landing a Repository can have, in the order its
// form offers them: the Project's Default Landing, or an Own Landing.
const LANDINGS: { value: RepositoryLanding; label: string }[] = [
  { value: "default", label: "Project default" },
  { value: "merge", label: "Merge" },
  { value: "merge_push", label: "Merge + Push" },
  { value: "branch", label: "Branch" },
  { value: "branch_push", label: "Branch + Push" },
  { value: "pull_request", label: "Pull Request" },
];
const LANDING_LABEL = Object.fromEntries(LANDINGS.map((l) => [l.value, l.label])) as Record<RepositoryLanding, string>;

// CODE_HOST_LABEL names a Repository's Code Host, worked out from its Clone
// URL; "" is a host Code Central doesn't support.
const CODE_HOST_LABEL: Record<CodeHost, string> = {
  github: "GitHub",
  azure_repos: "Azure Repos",
  "": "no supported Code Host",
};

// LANDING is the Default Landing section's id, where the Home Repository's
// Base is set. Like the section menu, jumping there leaves the URL, which
// holds the view, alone.
const LANDING = "project-options-default-landing";
const jumpToLanding = (e: MouseEvent<HTMLAnchorElement>) => {
  e.preventDefault();
  document.getElementById(LANDING)?.scrollIntoView?.();
};

// defaultLandingOf is the Project's Default Landing as a Repository's
// Landing names it.
const defaultLandingOf = ({ method, push }: Landing): RepositoryLanding =>
  method === "pull_request" ? method : push ? `${method}_push` : method;

// fieldsOf is r's fields as its edit form starts them: its Landing and
// Push remote as its own, or default and empty while it follows the
// Project's.
const fieldsOf = (r: RepositoryView): RepositoryFields => ({
  name: r.name,
  clone_url: r.clone_url,
  base: r.base,
  local_path: r.local_path,
  landing: r.own_landing ? r.landing : "default",
  push_remote: r.own_push_remote ? r.push_remote : "",
});

// RepositoriesSection is the Project's Repositories (Spec code-central-848)
// in landing order: the Home Repository, where the Factory runs, which is
// always there, and the others the human adds. Each row shows its Clone
// URL or local-only, its Landing and Push remote, its own or the
// Project's, and its CI choice (Spec code-central-890). Each can move up
// and down the landing order and is edited in place, the Home's Landing
// and Push remote alone, and each but the Home Repository is removed.
// Every refusal shows its reason and fix beside the Repository it is
// about.
export function RepositoriesSection({ project }: { project: ProjectRow }) {
  const api = useApi();
  const [editing, setEditing] = useState<string>();
  const repositories = project.repositories;
  const names = repositories.map((r) => r.name);
  const defaults = { landing: defaultLandingOf(project.landing), remote: project.landing.remote };
  // pushesNothing is Push on in some Landing while every Repository is
  // local-only, which is allowed: a Repository may have a remote later.
  const pushesNothing = repositories.every((r) => r.local_only) && repositories.some((r) => r.landing.endsWith("_push"));

  // move swaps the Repository at i with its neighbour by, -1 for up.
  const move = (i: number, by: -1 | 1) => {
    const order = [...names];
    [order[i], order[i + by]] = [order[i + by], order[i]];
    return api.setLandingOrder(project.key, order);
  };

  return (
    <fieldset
      id="project-options-repositories"
      className="project-options-section"
      disabled={project.archived}
    >
      <legend>Repositories</legend>
      <p className="project-options-hint">
        The repositories this Project&apos;s work changes, in landing order:
        accepted work lands in each in this order. The Home Repository is the
        one the Factory runs in, set up by <code>code-central init</code>.
      </p>
      {pushesNothing && (
        <p className="project-options-hint" role="note">
          No Repository has a Clone URL, so Push pushes nothing. It stays on,
          and so can CI Tracking, for when a Repository has one.
        </p>
      )}
      <ol className="project-options-repositories" aria-label="Repositories">
        {repositories.map((r, i) =>
          editing === r.name ? (
            <li key={r.name}>
              <RepositoryForm
                label={`Edit ${r.name}`}
                initial={fieldsOf(r)}
                submit="Save"
                save={({ clone_url, base, local_path, landing, push_remote }) =>
                  api
                    .editRepository(
                      project.key,
                      r.name,
                      // The Home's Clone URL is reported and its Base is
                      // the Project's: it takes its Landing alone.
                      r.home ? { landing, push_remote } : { clone_url, base, local_path, landing, push_remote },
                    )
                    .then(() => setEditing(undefined))
                }
                cancel={() => setEditing(undefined)}
                home={r.home}
                defaults={defaults}
              />
            </li>
          ) : (
            <RepositoryRow
              key={r.name}
              project={project.key}
              repository={r}
              several={repositories.length > 1}
              first={i === 0}
              last={i === repositories.length - 1}
              moveUp={() => move(i, -1)}
              moveDown={() => move(i, 1)}
              edit={() => setEditing(r.name)}
              remove={() => api.removeRepository(project.key, r.name)}
            />
          ),
        )}
      </ol>
      <RepositoryForm
        label="Add a Repository"
        initial={{ name: "", clone_url: "", base: "main", local_path: "", landing: "default", push_remote: "" }}
        submit="Add Repository"
        save={(r) => api.addRepository(project.key, r)}
        withName
        clearOnSave
        ciChoice={project.ci.tracking}
        defaults={defaults}
      />
    </fieldset>
  );
}

// RepositoryRow is one Repository: what it is, and its actions. Its clone
// shows only once the Project has several Repositories: with the Home
// Repository alone, the Factory has nothing to clone.
function RepositoryRow({
  project,
  repository: r,
  several,
  first,
  last,
  moveUp,
  moveDown,
  edit,
  remove,
}: {
  project: string;
  repository: RepositoryView;
  several: boolean;
  first: boolean;
  last: boolean;
  moveUp(): Promise<unknown>;
  moveDown(): Promise<unknown>;
  edit(): void;
  remove(): Promise<unknown>;
}) {
  const [error, setError] = useState<ApiError>();
  const id = useId();
  const act = (action: () => Promise<unknown>) => () => {
    setError(undefined);
    action().catch(setError);
  };
  // whyNotRemovable says why Remove is disabled, when it is.
  const whyNotRemovable = r.home
    ? "The Home Repository can't be removed: the Factory runs in it."
    : r.changed_by.length > 0
      ? `Unfinished Tickets have changed it: ${r.changed_by.join(", ")}. Finish or cancel them to remove it.`
      : undefined;

  return (
    <li aria-label={r.name}>
      <div className="project-options-repository">
        <strong>{r.name}</strong>
        {r.home && <span className="project-options-home-badge">Home</span>}
      </div>
      <dl className="project-options-repository-details">
        <dt>Clone URL</dt>
        <dd>
          {r.clone_url ||
            (!r.local_only
              ? "Not reported yet: start the Factory once and it reports its Push remote."
              : r.home
                ? "local-only: its clone has no Push remote. Add one and restart the Factory to push it."
                : "local-only: never cloned or pushed")}
        </dd>
        <dt>Code Host</dt>
        <dd>{r.clone_url && CODE_HOST_LABEL[r.code_host]}</dd>
        <dt>Base</dt>
        <dd>
          {r.base}
          {r.home && (
            <>
              {" "}
              (the Project&apos;s Base, set in{" "}
              <a href={`#${LANDING}`} onClick={jumpToLanding}>
                Default Landing
              </a>
              )
            </>
          )}
        </dd>
        {!r.home && (
          <>
            <dt>Local path</dt>
            <dd>{r.local_path || "Beside the Home Repository"}</dd>
          </>
        )}
        <dt>Landing</dt>
        <dd>
          {LANDING_LABEL[r.landing]} ({r.own_landing ? "its own" : "Project default"})
        </dd>
        <dt>Push remote</dt>
        <dd>
          {r.push_remote} ({r.own_push_remote ? "its own" : "Project's"})
        </dd>
        <dt>CI</dt>
        <dd>
          <RepositoryCI project={project} entry={r.ci} />
        </dd>
        {several && (
          <>
            <dt>Clone</dt>
            <dd>
              <CloneStatus clone={r.clone} />
            </dd>
          </>
        )}
      </dl>
      {r.local_only_under_pull_request && (
        <p className="warning" role="alert">
          local-only under Pull Request: nothing lands in {r.name} until it has another Landing or a remote.
          <br />
          Give {r.name} {r.own_landing ? "another Landing" : "an Own Landing"} such as Merge,{" "}
          {r.home ? "or add a Push remote to its clone and restart the Factory." : "or a Clone URL."}
        </p>
      )}
      <div className="project-options-actions">
        <Button type="button" aria-label={`Move ${r.name} up`} disabled={first} onClick={act(moveUp)}>
          Move up
        </Button>
        <Button type="button" aria-label={`Move ${r.name} down`} disabled={last} onClick={act(moveDown)}>
          Move down
        </Button>
        <Button type="button" aria-label={`Edit ${r.name}`} onClick={edit}>
          Edit
        </Button>
        <Button
          type="button"
          aria-label={`Remove ${r.name}`}
          aria-describedby={whyNotRemovable ? `${id}-not-removable` : undefined}
          disabled={whyNotRemovable !== undefined}
          onClick={act(remove)}
        >
          Remove
        </Button>
      </div>
      {whyNotRemovable && (
        <p className="project-options-hint" id={`${id}-not-removable`}>
          {whyNotRemovable}
        </p>
      )}
      {error && <Refusal id={`${id}-error`} error={error} />}
    </li>
  );
}

// CloneStatus is a Repository's clone as its Factory last reported it:
// cloned at its path, or missing there with the problem.
function CloneStatus({ clone }: { clone?: CloneView }) {
  if (!clone) return <>Not reported: a running Factory reports it on each poll.</>;
  if (clone.state === "ready")
    return (
      <>
        Cloned at <code>{clone.path}</code>
      </>
    );
  return (
    <span className="project-options-clone-missing">
      Missing at <code>{clone.path}</code>: {clone.problem}
    </span>
  );
}

// RepositoryFields are a Repository's fields as its form edits them.
type RepositoryFields = Required<Omit<RepositoryAdd, "ci">>;

// RepositoryForm is a Repository's fields: adding one, with its name, or
// editing one in place, whose name never changes; the Home's, its
// Landing and Push remote alone. defaults are the Project's Default
// Landing and Push remote, which it follows while it has none of its
// own. With ciChoice, while CI Tracking is on, adding one also chooses
// whether CI Tracking watches it, as its Clone URL names it, or leaves it
// Not watched.
function RepositoryForm(props: {
  label: string;
  initial: RepositoryFields;
  submit: string;
  save(r: RepositoryAdd): Promise<unknown>;
  cancel?(): void;
  withName?: boolean;
  clearOnSave?: boolean;
  ciChoice?: boolean;
  home?: boolean;
  defaults: { landing: RepositoryLanding; remote: string };
}) {
  const { label, initial, submit, save, cancel, withName, clearOnSave, ciChoice, home, defaults } = props;
  const [draft, setDraft] = useState<RepositoryFields>(initial);
  const [watchCI, setWatchCI] = useState(true);
  const [error, setError] = useState<ApiError>();
  const id = useId();

  const edit = (patch: Partial<RepositoryFields>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
  };
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    save(showCIChoice && !watchCI ? { ...draft, ci: { not_watched: true } } : draft).then(
      () => {
        if (clearOnSave) {
          setDraft(initial);
          setWatchCI(true);
        }
      },
      (e: ApiError) => setError(e),
    );
  };
  const invalid = (field: string) =>
    error?.field === field
      ? { "aria-invalid": true, "aria-describedby": `${id}-error` }
      : undefined;
  const hasCloneURL = draft.clone_url.trim() !== "";
  const filled = home || (draft.name.trim() !== "" && (hasCloneURL || draft.local_path.trim() !== ""));
  // A Repository with no Clone URL is never watched, so there is nothing to choose.
  const showCIChoice = ciChoice && hasCloneURL;

  return (
    <form aria-label={label} className="project-options-repository-form" onSubmit={onSubmit}>
      {withName && (
        <Field
          label="Name"
          hintId={`${id}-name`}
          hint="Its Repository Name, fixed once added: lowercase letters, digits, -, _ and ."
        >
          <input
            value={draft.name}
            placeholder="web"
            onChange={(e) => edit({ name: e.target.value })}
            aria-describedby={`${id}-name`}
            {...invalid("name")}
          />
        </Field>
      )}
      {!home && (
        <>
          <Field
            label="Clone URL"
            hintId={`${id}-clone-url`}
            hint="Leave empty for a local-only repository: give its local path. It is never cloned or pushed."
          >
            <input
              value={draft.clone_url}
              placeholder="git@github.com:acme/web.git"
              onChange={(e) => edit({ clone_url: e.target.value })}
              aria-describedby={`${id}-clone-url`}
              {...invalid("clone_url")}
            />
          </Field>
          <Field label="Base">
            <input
              value={draft.base}
              onChange={(e) => edit({ base: e.target.value })}
              {...invalid("base")}
            />
          </Field>
          <Field
            label="Local path"
            hintId={`${id}-local-path`}
            hint="An existing clone's full path. Leave it empty to have the Factory clone it beside the Home Repository."
          >
            <input
              value={draft.local_path}
              onChange={(e) => edit({ local_path: e.target.value })}
              aria-describedby={`${id}-local-path`}
              {...invalid("local_path")}
            />
          </Field>
        </>
      )}
      <Field
        label="Landing"
        hintId={`${id}-landing`}
        hint="Project default follows the Default Landing below; any other is its Own Landing. A local-only Repository is never pushed, and can't be under Pull Request."
      >
        <Select
          value={draft.landing}
          options={LANDINGS.map((l) =>
            l.value === "default" ? { ...l, label: `Project default (${LANDING_LABEL[defaults.landing]})` } : l,
          )}
          onChange={(v) => edit({ landing: v || "default" })}
          aria-describedby={`${id}-landing`}
          {...invalid("landing")}
        />
      </Field>
      <Field
        label="Push remote"
        hintId={`${id}-push-remote`}
        hint={`The git remote it is pushed to; leave it empty for the Project's, ${defaults.remote}.`}
      >
        <input
          value={draft.push_remote}
          placeholder={defaults.remote}
          onChange={(e) => edit({ push_remote: e.target.value })}
          aria-describedby={`${id}-push-remote`}
          {...invalid("push_remote")}
        />
      </Field>
      {showCIChoice && (
        <label className="project-options-check">
          <input
            type="checkbox"
            checked={watchCI}
            onChange={(e) => {
              setWatchCI(e.target.checked);
              setError(undefined);
            }}
            {...invalid("ci.repository")}
          />{" "}
          Watch its CI, as its Clone URL names it; unchecked, it is Not watched
        </label>
      )}
      {error && <Refusal id={`${id}-error`} error={error} />}
      <div className="project-options-actions">
        <Button type="submit" variant="primary" disabled={!filled}>
          {submit}
        </Button>
        {cancel && (
          <Button type="button" onClick={cancel}>
            Cancel
          </Button>
        )}
      </div>
    </form>
  );
}
