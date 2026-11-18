import { useEffect, useId, useState, type FormEvent } from "react";
import { useApi } from "../live";
import type { ApiError, CISettings, ProjectRow, RepositoryView } from "../api/types";
import { Button } from "../controls/Button";
import { Field } from "../controls/Field";
import { AutoFixBlock } from "./AutoFixBlock";
import { CIRules } from "./CIRules";
import { Refusal } from "./Refusal";
import { WatcherStatus } from "./WatcherStatus";

// CIDraft is the CI section's form: Ignored workflows as typed, one string.
interface CIDraft {
  tracking: boolean;
  patEnv: string;
  ignored: string;
}

// ciDraft is the form for ci as stored.
function ciDraft(ci: CISettings): CIDraft {
  return {
    tracking: ci.tracking,
    patEnv: ci.pat_env || "ADO_PAT",
    ignored: ci.ignored_workflows.join(", "),
  };
}

// workflowNames splits Ignored workflows as typed, by commas or spaces.
const workflowNames = (ignored: string) =>
  ignored.split(/[\s,]+/).filter(Boolean);

// watched is the Repositories whose CI choice is a CI Provider, not Not
// watched.
const watched = (repositories: RepositoryView[]) =>
  repositories.filter((r) => !r.ci.not_watched);

// watching names what the CI Watcher watches: the one watched
// Repository's GitHub repository or Azure DevOps project, or how many.
function watching(repositories: RepositoryView[]): string {
  const ws = watched(repositories);
  if (ws.length !== 1) return `its ${ws.length} watched Repositories`;
  const { ci } = ws[0];
  return (ci.provider === "azure_pipelines" ? ci.project_url : ci.repository) ?? "";
}

// ignoredHint is the Ignored workflows' hint for the CI Providers the
// watched Repositories are on.
function ignoredHint(repositories: RepositoryView[]): string {
  const providers = new Set(watched(repositories).map((r) => r.ci.provider));
  const names = !providers.has("azure_pipelines")
    ? "Workflow file names, such as nightly.yml,"
    : providers.size === 1
      ? "Pipeline names, such as Nightly build,"
      : "Workflow file names, such as nightly.yml, or pipeline names, such as Nightly build,";
  return `${names} separated by commas: their runs never count towards a Ticket's CI result.`;
}

// CISection is the Project's CI settings that are every Repository's
// (Spec code-central-890): CI Tracking, the PAT environment variable
// while any Repository is on Azure Pipelines, the Ignored workflows and
// the CI Rules. Each Repository's CI choice is in its row under
// Repositories. Like the Default Landing it follows the Project as read.
export function CISection({ project }: { project: ProjectRow }) {
  const api = useApi();
  const stored = ciDraft(project.ci);
  const [draft, setDraft] = useState<CIDraft>(stored);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  const { tracking, patEnv, ignored } = stored;
  useEffect(() => setDraft({ tracking, patEnv, ignored }), [tracking, patEnv, ignored]);

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
    draft.patEnv !== patEnv ||
    workflowNames(draft.ignored).join(",") !==
      workflowNames(ignored).join(",");
  const invalid = (field: string) =>
    error?.field === field
      ? { "aria-invalid": true, "aria-describedby": `${id}-error` }
      : undefined;
  const azurePipelines = watched(project.repositories).some((r) => r.ci.provider === "azure_pipelines");

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
            Each Repository&apos;s CI, or Not watched, is set in its row under
            Repositories.
          </p>
        </div>
        {azurePipelines && (
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
        )}
        <Field label="Ignored workflows" hintId={`${id}-ignored`} hint={ignoredHint(project.repositories)}>
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
      {project.ci.tracking && <AutoFixBlock project={project} />}
      {project.ci.tracking && (
        <WatcherStatus name="CI Watcher" repository={watching(project.repositories)} watcher={project.ci.watcher} />
      )}
      <CIRules project={project} />
    </fieldset>
  );
}
