import { useId, useState, type FormEvent } from "react";
import { useApi } from "../live";
import type { ApiError, CIProvider, CIRepositoryView } from "../api/types";
import { Button } from "../controls/Button";
import { Field } from "../controls/Field";
import { Select } from "../controls/Select";
import { Refusal } from "./Refusal";

// NOT_WATCHED is the CI choice that leaves a Repository's CI untracked.
const NOT_WATCHED = "not_watched";

const CHOICES: { value: CIProvider | typeof NOT_WATCHED; label: string }[] = [
  { value: "github_actions", label: "GitHub Actions" },
  { value: "azure_pipelines", label: "Azure Pipelines" },
  { value: NOT_WATCHED, label: "Not watched" },
];

// ciTarget is what a CI entry watches, as its row names it.
export function ciTarget(e: Pick<CIRepositoryView, "not_watched" | "provider" | "repository" | "project_url">): string {
  if (e.not_watched) return "Not watched";
  return e.provider === "azure_pipelines"
    ? `Azure Pipelines on ${e.project_url || "no project yet"}`
    : `GitHub Actions on ${e.repository || "no repository yet"}`;
}

// RepositoryCI is a Repository's CI choice in its row (Spec
// code-central-890), the Home's included: its CI Provider and what that
// watches, or Not watched, edited in place. The CI section holds what
// is every Repository's: CI Tracking, the PAT environment variable, the
// Ignored workflows and the CI Rules.
export function RepositoryCI({ project, entry }: { project: string; entry: CIRepositoryView }) {
  const [editing, setEditing] = useState(false);
  if (editing) return <RepositoryCIForm project={project} entry={entry} close={() => setEditing(false)} />;
  return (
    <>
      {ciTarget(entry)}{" "}
      <Button type="button" onClick={() => setEditing(true)} aria-label={`Edit ${entry.name}'s CI`}>
        Edit
      </Button>
    </>
  );
}

// RepositoryCIForm is a Repository's CI choice being edited, starting
// from its entry as read.
function RepositoryCIForm({ project, entry, close }: { project: string; entry: CIRepositoryView; close(): void }) {
  const api = useApi();
  const id = useId();
  const [choice, setChoice] = useState<string>(entry.not_watched ? NOT_WATCHED : (entry.provider ?? "github_actions"));
  const [repository, setRepository] = useState(entry.repository ?? "");
  const [projectURL, setProjectURL] = useState(entry.project_url ?? "");
  const [error, setError] = useState<ApiError>();
  const invalid = (field: string) => (error?.field === field ? { "aria-invalid": true, "aria-describedby": `${id}-error` } : undefined);

  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    const change =
      choice === NOT_WATCHED
        ? { not_watched: true }
        : choice === "azure_pipelines"
          ? { provider: "azure_pipelines" as CIProvider, project_url: projectURL }
          : { provider: "github_actions" as CIProvider, repository };
    api.setRepositoryCI(project, entry.name, change).then(close, (e: ApiError) => setError(e));
  };

  return (
    <form aria-label={`${entry.name}'s CI`} onSubmit={save}>
      <label className="field">
        CI{" "}
        <Select
          value={choice}
          options={CHOICES}
          onChange={(v) => {
            setChoice(v);
            setError(undefined);
          }}
        />
      </label>
      {choice === "github_actions" && (
        <Field label="GitHub repository">
          <input value={repository} placeholder="owner/name" onChange={(e) => setRepository(e.target.value)} {...invalid("repository")} />
        </Field>
      )}
      {choice === "azure_pipelines" && (
        <Field label="Azure DevOps project URL">
          <input
            value={projectURL}
            placeholder="https://dev.azure.com/org/project"
            onChange={(e) => setProjectURL(e.target.value)}
            {...invalid("project_url")}
          />
        </Field>
      )}
      {error && <Refusal id={`${id}-error`} error={error} />}
      <div className="project-options-actions">
        <Button type="submit" variant="primary">
          Save
        </Button>
        <Button type="button" onClick={close}>
          Cancel
        </Button>
      </div>
    </form>
  );
}
