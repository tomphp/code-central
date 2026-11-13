import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import type {
  ApiError,
  CIProvider,
  CISettings,
  ProjectRow,
} from "../api/types";
import { Field } from "../controls/Field";
import { Select } from "../controls/Select";
import { azureProjectURL } from "./azureProjectURL";
import { CIRules } from "./CIRules";
import { githubRepository } from "./githubRepository";
import { Refusal } from "./Refusal";
import { WatcherStatus } from "./WatcherStatus";

const PROVIDERS: { provider: CIProvider; label: string }[] = [
  { provider: "github_actions", label: "GitHub Actions" },
  { provider: "azure_pipelines", label: "Azure Pipelines" },
];

// CIDraft is the CI section's form: Ignored workflows as typed, one string.
interface CIDraft {
  tracking: boolean;
  provider: CIProvider;
  repository: string;
  projectURL: string;
  patEnv: string;
  ignored: string;
}

// ciDraft is the form for ci as stored, its repository prefilled from a
// GitHub Push remote while none is set, and, for Azure Pipelines, its
// project URL from an Azure Repos one.
function ciDraft(ci: CISettings): CIDraft {
  return {
    tracking: ci.tracking,
    provider: ci.provider,
    repository: ci.repository || githubRepository(ci.push_remote_url) || "",
    projectURL:
      ci.project_url ||
      (ci.provider === "azure_pipelines"
        ? azureProjectURL(ci.push_remote_url)
        : undefined) ||
      "",
    patEnv: ci.pat_env || "ADO_PAT",
    ignored: ci.ignored_workflows.join(", "),
  };
}

// workflowNames splits Ignored workflows as typed, by commas or spaces.
const workflowNames = (ignored: string) =>
  ignored.split(/[\s,]+/).filter(Boolean);

// CISection is the Project's CI settings: CI Tracking, its CI Provider,
// the GitHub repository or Azure DevOps project it watches, and the Ignored workflows. Like the Landing Method it follows the Project as read.
export function CISection({ api, project }: { api: Api; project: ProjectRow }) {
  const stored = ciDraft(project.ci);
  const prefilled = !project.ci.repository && stored.repository !== "";
  const [draft, setDraft] = useState<CIDraft>(stored);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const remoteURL = azureProjectURL(project.ci.push_remote_url);
  const urlPrefilled =
    !project.ci.project_url &&
    remoteURL !== undefined &&
    draft.projectURL === remoteURL;
  const id = useId();
  const {
    tracking,
    provider,
    repository,
    projectURL,
    patEnv,
    ignored,
  } = stored;
  useEffect(
    () =>
      setDraft({
        tracking,
        provider,
        repository,
        projectURL,
        patEnv,
        ignored,
          }),
    [
      tracking,
      provider,
      repository,
      projectURL,
      patEnv,
      ignored,
      ],
  );

  const edit = (patch: Partial<CIDraft>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
    setSaved(false);
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    const change = {
      tracking: draft.tracking,
      provider: draft.provider,
      repository: draft.repository,
      project_url: draft.projectURL,
      pat_env: draft.patEnv,
      ignored_workflows: workflowNames(draft.ignored),
    };
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
    draft.projectURL !== projectURL ||
    draft.patEnv !== patEnv ||
    workflowNames(draft.ignored).join(",") !==
      workflowNames(ignored).join(",");
  const invalid = (field: string) =>
    error?.field === field
      ? { "aria-invalid": true, "aria-describedby": `${id}-error` }
      : undefined;

  return (
    <fieldset id="project-options-ci" className="project-options-section" disabled={project.archived}>
      <legend>CI</legend>
      <form onSubmit={save}>
        <div className="project-options-choice">
          <label>
            <input
              type="checkbox"
              checked={draft.tracking}
              onChange={(e) => edit({ tracking: e.target.checked })}
              aria-describedby={
                error?.field === "tracking"
                  ? `${id}-tracking ${id}-error`
                  : `${id}-tracking`
              }
              aria-invalid={error?.field === "tracking" || undefined}
            />{" "}
            CI Tracking
          </label>
          <p className="project-options-hint" id={`${id}-tracking`}>
            CI Tracking watches the CI of what the Factory pushes, through your
            existing <code>gh</code> login. Code Central stores no GitHub token.
          </p>
        </div>
        <label className="project-options-field">
          CI Provider{" "}
          <Select
            value={draft.provider}
            options={PROVIDERS.map((pr) => ({ value: pr.provider, label: pr.label }))}
            onChange={(v) => {
              // Each provider keeps its own values; Azure Pipelines starts from the Push remote.
              const next = v as CIProvider;
              edit({
                provider: next,
                ...(next === "azure_pipelines" &&
                !project.ci.project_url &&
                draft.projectURL === "" &&
                remoteURL
                  ? { projectURL: remoteURL }
                  : {}),
              });
            }}
          />
        </label>
        {draft.provider === "github_actions" && (
          <>
            <Field
              label="GitHub repository"
              hintId={`${id}-repository`}
              hint={
                <>
              {prefilled
                ? `From the Push remote ${project.ci.push_remote_url}. `
                : ""}
              The repository to watch, as owner/name; CI Tracking needs one.
                </>
              }
            >
              <input
                value={draft.repository}
                placeholder="owner/name"
                onChange={(e) => edit({ repository: e.target.value })}
                aria-describedby={`${id}-repository`}
                {...invalid("repository")}
              />
            </Field>
          </>
        )}
        {draft.provider === "azure_pipelines" && (
          <>
            <Field
              label="Azure DevOps project URL"
              hintId={`${id}-project-url`}
              hint={
                <>
              {urlPrefilled
                ? `From the Push remote ${project.ci.push_remote_url}. `
                : ""}
              The Azure DevOps project to watch, as its URL, such as
              https://dev.azure.com/acme/web; CI Tracking needs one.
                </>
              }
            >
              <input
                value={draft.projectURL}
                placeholder="https://dev.azure.com/org/project"
                onChange={(e) => edit({ projectURL: e.target.value })}
                aria-describedby={`${id}-project-url`}
                {...invalid("project_url")}
              />
            </Field>
            <Field
              label="PAT environment variable"
              hintId={`${id}-pat-env`}
              hint={
                <>
              The environment variable that holds an Azure DevOps PAT with
              Build: Read &amp; execute, in the service&apos;s environment. Code
              Central keeps the name, never the PAT.
                </>
              }
            >
              <input
                value={draft.patEnv}
                placeholder="ADO_PAT"
                onChange={(e) => edit({ patEnv: e.target.value })}
                aria-describedby={`${id}-pat-env`}
                {...invalid("pat_env")}
              />
            </Field>
          </>
        )}
        <Field
          label="Ignored workflows"
          hintId={`${id}-ignored`}
          hint={
            <>
          {draft.provider === "azure_pipelines"
            ? "Pipeline names, such as Nightly build, separated by commas: their runs never count towards a Ticket's CI result."
            : "Workflow file names, such as nightly.yml, separated by commas: their runs never count towards a Ticket's CI result."}
            </>
          }
        >
          <input
            value={draft.ignored}
            placeholder="nightly.yml, preview.yml"
            onChange={(e) => edit({ ignored: e.target.value })}
            aria-describedby={`${id}-ignored`}
            {...invalid("ignored_workflows")}
          />
        </Field>
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
      {project.ci.tracking && (
        <WatcherStatus
          name="CI Watcher"
          repository={
            project.ci.provider === "azure_pipelines"
              ? project.ci.project_url
              : project.ci.repository
          }
          watcher={project.ci.watcher}
        />
      )}
      <CIRules api={api} project={project} />
    </fieldset>
  );
}
