import { useEffect, useId, useState, type FormEvent } from "react";
import { useApi } from "../live";
import type {
  ApiError,
  AzureBoardsChange,
  AzureBoardsMode,
  ProjectRow,
} from "../api/types";
import { Button } from "../controls/Button";
import { Field } from "../controls/Field";
import { azureProjectURL } from "./azureProjectURL";
import { Refusal } from "./Refusal";
import { WatcherStatus } from "./WatcherStatus";

const MODES: { mode: AzureBoardsMode; label: string; hint: string }[] = [
  {
    mode: "saved_query",
    label: "Saved query",
    hint: "The work items of a flat-list query saved in Azure Boards.",
  },
  {
    mode: "wiql",
    label: "WIQL filter",
    hint: "The work items of an Azure DevOps project that a WHERE clause picks.",
  },
];

// projectURLPrefill is the Azure DevOps project URL to offer while none is
// set: the Azure Pipelines CI setting's, else an Azure Repos Repository's
// Clone URL's.
function projectURLPrefill(project: ProjectRow): string {
  if (project.ci.provider === "azure_pipelines" && project.ci.project_url) {
    return project.ci.project_url;
  }
  for (const r of project.repositories) {
    const url = azureProjectURL(r.clone_url);
    if (url) return url;
  }
  return "";
}

// azureDraft is the form for Azure Boards as stored, its project URL
// prefilled while none is set.
function azureDraft(project: ProjectRow): AzureBoardsChange {
  const a = project.azure_boards;
  return {
    on: a.on,
    mode: a.mode,
    query_url: a.query_url,
    project_url: a.project_url || projectURLPrefill(project),
    where: a.where,
    pat_env: a.pat_env,
    new_issues_need_me: a.new_issues_need_me,
    hide_code_central: a.hide_code_central,
  };
}

// AzureBoardsSection is the Project's Azure Boards settings (Spec
// code-central-894): the switch, the mode (a Saved query's URL, or a WIQL
// filter's project URL and WHERE clause), the PAT variable and New issues
// need me and Hide Code Central. Turning it on, or changing a setting while it is on, runs the
// check, whose refusal shows in place with its fix; while it is on, the
// Work Item Watcher's status shows below. Like the other sections it
// follows the Project as read.
export function AzureBoardsSection({ project }: { project: ProjectRow }) {
  const api = useApi();
  const azure = project.azure_boards;
  const stored = azureDraft(project);
  const prefilled = !azure.project_url && stored.project_url !== "";
  const [draft, setDraft] = useState<AzureBoardsChange>(stored);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  const { on, mode, query_url, project_url, where, pat_env, new_issues_need_me, hide_code_central } = stored;
  useEffect(
    () => setDraft({ on, mode, query_url, project_url, where, pat_env, new_issues_need_me, hide_code_central }),
    [on, mode, query_url, project_url, where, pat_env, new_issues_need_me, hide_code_central],
  );

  const edit = (patch: Partial<AzureBoardsChange>) => {
    setDraft((d) => ({ ...d, ...patch }));
    setError(undefined);
    setSaved(false);
  };
  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    api.setAzureBoards(project.key, draft).then(
      (v) => {
        setDraft(azureDraft({ ...project, azure_boards: v.azure_boards }));
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed =
    draft.on !== on ||
    draft.mode !== mode ||
    draft.query_url.trim() !== query_url ||
    draft.project_url.trim() !== project_url ||
    draft.where.trim() !== where ||
    draft.pat_env.trim() !== pat_env ||
    draft.new_issues_need_me !== new_issues_need_me ||
    draft.hide_code_central !== hide_code_central;
  const describedBy = (field: string, hint: string) =>
    error?.field === field ? `${hint} ${id}-error` : hint;
  const invalid = (field: string) => error?.field === field || undefined;
  const watching = azure.mode === "wiql" ? azure.project_url : azure.query_url;

  return (
    <fieldset id="project-options-azure-boards" className="project-options-section" disabled={project.archived}>
      <legend>Azure Boards</legend>
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
            Azure Boards
          </label>
          <p className="project-options-hint" id={`${id}-on`}>
            Each work item the query or filter picks arrives as an Incoming
            Issue, a Draft Ticket to triage, and Code Central writes the
            work&apos;s progress back to it as tags and comments, with an
            Azure DevOps PAT. Code Central keeps the variable&apos;s name,
            never the PAT.
          </p>
        </div>
        <div role="radiogroup" aria-label="Azure Boards mode">
          {MODES.map((m) => (
            <div key={m.mode} className="project-options-choice">
              <label>
                <input
                  type="radio"
                  name={`${id}-mode`}
                  checked={draft.mode === m.mode}
                  onChange={() => edit({ mode: m.mode })}
                  aria-describedby={`${id}-${m.mode}`}
                />{" "}
                {m.label}
              </label>
              <p className="project-options-hint" id={`${id}-${m.mode}`}>
                {m.hint}
              </p>
            </div>
          ))}
        </div>
        {draft.mode === "saved_query" && (
          <Field
            label="Query URL"
            hintId={`${id}-query-url`}
            hint="The saved query's URL, as the browser shows it. It must be a flat list: tree and one-hop queries are refused."
          >
            <input
              value={draft.query_url}
              placeholder="https://dev.azure.com/org/project/_queries/query/…"
              onChange={(e) => edit({ query_url: e.target.value })}
              aria-describedby={describedBy("query_url", `${id}-query-url`)}
              aria-invalid={invalid("query_url")}
            />
          </Field>
        )}
        {draft.mode === "wiql" && (
          <>
            <Field
              label="Azure DevOps project URL"
              hintId={`${id}-project-url`}
              hint={
                <>
                  {prefilled && draft.project_url === stored.project_url
                    ? "From this Project's Azure Repos Repository or Azure Pipelines setting. "
                    : ""}
                  The Azure DevOps project the filter runs in, as its URL, such
                  as https://dev.azure.com/acme/web.
                </>
              }
            >
              <input
                value={draft.project_url}
                placeholder="https://dev.azure.com/org/project"
                onChange={(e) => edit({ project_url: e.target.value })}
                aria-describedby={describedBy("project_url", `${id}-project-url`)}
                aria-invalid={invalid("project_url")}
              />
            </Field>
            <Field
              label="WHERE clause"
              hintId={`${id}-where`}
              hint="A WIQL WHERE clause, without the WHERE keyword."
            >
              <textarea
                rows={4}
                value={draft.where}
                placeholder="[System.State] = 'Approved' AND [System.AssignedTo] = @Me"
                onChange={(e) => edit({ where: e.target.value })}
                aria-describedby={describedBy("where", `${id}-where`)}
                aria-invalid={invalid("where")}
              />
            </Field>
          </>
        )}
        <Field
          label="PAT environment variable"
          hintId={`${id}-pat-env`}
          hint="The environment variable that holds an Azure DevOps PAT with Work Items: Read & write, in the service's environment."
        >
          <input
            value={draft.pat_env}
            placeholder="ADO_PAT"
            onChange={(e) => edit({ pat_env: e.target.value })}
            aria-describedby={describedBy("pat_env", `${id}-pat-env`)}
            aria-invalid={invalid("pat_env")}
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
        <div className="project-options-choice">
          <label>
            <input
              type="checkbox"
              checked={draft.hide_code_central}
              onChange={(e) => edit({ hide_code_central: e.target.checked })}
              aria-describedby={`${id}-hide`}
            />{" "}
            Hide Code Central
          </label>
          <p className="project-options-hint" id={`${id}-hide`}>
            Write-back still posts every comment, but writes no{" "}
            <code>code-central:</code> tag, no link back and no Spec or Ticket
            IDs. Tags and comments already on work items stay as they are.
          </p>
        </div>
        {azure.spec && <p className="muted">Incoming Issues arrive on {azure.spec}.</p>}
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
      {azure.on && (
        <WatcherStatus name="Work Item Watcher" repository={watching} watcher={azure.watcher} />
      )}
    </fieldset>
  );
}
