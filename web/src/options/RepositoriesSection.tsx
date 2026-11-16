import { useId, useState, type FormEvent, type MouseEvent } from "react";
import { useApi } from "../live";
import type {
  ApiError,
  ProjectRow,
  RepositoryAdd,
  RepositoryView,
  CloneView,
  CodeHost,
} from "../api/types";
import { Button } from "../controls/Button";
import { Field } from "../controls/Field";
import { Refusal } from "./Refusal";

// CODE_HOST_LABEL names a Repository's Code Host, worked out from its Clone
// URL; "" is a host Code Central doesn't support.
const CODE_HOST_LABEL: Record<CodeHost, string> = {
  github: "GitHub",
  azure_repos: "Azure Repos",
  "": "no supported Code Host",
};

// LANDING is the Landing Method section's id, where the Home Repository's
// Base is set. Like the section menu, jumping there leaves the URL, which
// holds the view, alone.
const LANDING = "project-options-landing-method";
const jumpToLanding = (e: MouseEvent<HTMLAnchorElement>) => {
  e.preventDefault();
  document.getElementById(LANDING)?.scrollIntoView?.();
};

// RepositoriesSection is the Project's Repositories (Spec code-central-848)
// in landing order: the Home Repository, where the Factory runs, which is
// always there, and the others the human adds. Each can move up and down
// the landing order, and each but the Home Repository is edited in place
// and removed. Every refusal shows its reason and fix beside the
// Repository it is about.
export function RepositoriesSection({ project }: { project: ProjectRow }) {
  const api = useApi();
  const [editing, setEditing] = useState<string>();
  const repositories = project.repositories;
  const names = repositories.map((r) => r.name);

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
      <ol className="project-options-repositories" aria-label="Repositories">
        {repositories.map((r, i) =>
          editing === r.name ? (
            <li key={r.name}>
              <RepositoryForm
                label={`Edit ${r.name}`}
                initial={r}
                submit="Save"
                save={({ clone_url, base, local_path }) =>
                  api
                    .editRepository(project.key, r.name, { clone_url, base, local_path })
                    .then(() => setEditing(undefined))
                }
                cancel={() => setEditing(undefined)}
              />
            </li>
          ) : (
            <RepositoryRow
              key={r.name}
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
        initial={{ name: "", clone_url: "", base: "main", local_path: "" }}
        submit="Add Repository"
        save={(r) => api.addRepository(project.key, r)}
        withName
        clearOnSave
      />
    </fieldset>
  );
}

// RepositoryRow is one Repository: what it is, and its actions. Its clone
// shows only once the Project has several Repositories: with the Home
// Repository alone, the Factory has nothing to clone.
function RepositoryRow({
  repository: r,
  several,
  first,
  last,
  moveUp,
  moveDown,
  edit,
  remove,
}: {
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
            (r.home ? "Not reported yet: start the Factory once and it reports its Push remote." : "")}
        </dd>
        <dt>Code Host</dt>
        <dd>{CODE_HOST_LABEL[r.code_host]}</dd>
        <dt>Base</dt>
        <dd>
          {r.base}
          {r.home && (
            <>
              {" "}
              (the Project&apos;s Base, set in{" "}
              <a href={`#${LANDING}`} onClick={jumpToLanding}>
                Landing Method
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
        {several && (
          <>
            <dt>Clone</dt>
            <dd>
              <CloneStatus clone={r.clone} />
            </dd>
          </>
        )}
      </dl>
      <div className="project-options-actions">
        <Button type="button" aria-label={`Move ${r.name} up`} disabled={first} onClick={act(moveUp)}>
          Move up
        </Button>
        <Button type="button" aria-label={`Move ${r.name} down`} disabled={last} onClick={act(moveDown)}>
          Move down
        </Button>
        {!r.home && (
          <Button type="button" aria-label={`Edit ${r.name}`} onClick={edit}>
            Edit
          </Button>
        )}
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

// RepositoryForm is a Repository's fields: adding one, with its name, or
// editing one in place, whose name never changes.
function RepositoryForm(props: {
  label: string;
  initial: Required<RepositoryAdd>;
  submit: string;
  save(r: Required<RepositoryAdd>): Promise<unknown>;
  cancel?(): void;
  withName?: boolean;
  clearOnSave?: boolean;
}) {
  const { label, initial, submit, save, cancel, withName, clearOnSave } = props;
  const [draft, setDraft] = useState<Required<RepositoryAdd>>({
    name: initial.name,
    clone_url: initial.clone_url,
    base: initial.base,
    local_path: initial.local_path,
  });
  const [error, setError] = useState<ApiError>();
  const id = useId();

  const edit = (patch: Partial<RepositoryAdd>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
  };
  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    save(draft).then(
      () => clearOnSave && setDraft(initial),
      (e: ApiError) => setError(e),
    );
  };
  const invalid = (field: string) =>
    error?.field === field
      ? { "aria-invalid": true, "aria-describedby": `${id}-error` }
      : undefined;
  const filled = draft.name.trim() !== "" && draft.clone_url.trim() !== "";

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
      <Field label="Clone URL">
        <input
          value={draft.clone_url}
          placeholder="git@github.com:acme/web.git"
          onChange={(e) => edit({ clone_url: e.target.value })}
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
