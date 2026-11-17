import { useId, useState, type FormEvent } from "react";
import { useApi } from "../live";
import type { ApiError, CIProvider, CIRepositoryView, ProjectRow } from "../api/types";
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

// target is what a CI entry watches, as its row names it.
export function ciTarget(e: Pick<CIRepositoryView, "not_watched" | "provider" | "repository" | "project_url">): string {
  if (e.not_watched) return "Not watched";
  return e.provider === "azure_pipelines"
    ? `Azure Pipelines on ${e.project_url || "no project yet"}`
    : `GitHub Actions on ${e.repository || "no repository yet"}`;
}

// CIRepositories is the CI of each of the Project's Repositories (Spec
// code-central-848), in landing order: the Home Repository's is the
// settings above, and each other Repository's is its own CI Provider and
// what it watches, or Not watched, edited in place. The PAT environment
// variable, the Ignored workflows and the CI Rules are every
// Repository's. It shows only while the Project has more than one
// Repository.
export function CIRepositories({ project }: { project: ProjectRow }) {
  const entries = project.ci.repositories;
  if (!entries || entries.length === 0) return null;
  const home = project.repositories.find((r) => r.home)?.name ?? project.key;
  return (
    <section aria-label="CI per Repository" className="project-options-ci-repositories">
      <h4>Each Repository&apos;s CI</h4>
      <ul>
        <li>
          <strong>{home}</strong>: {ciTarget({ not_watched: false, ...project.ci })}, the Home Repository&apos;s, set above
        </li>
        {entries.map((e) => (
          <CIRepositoryRow key={e.name} project={project.key} entry={e} />
        ))}
      </ul>
    </section>
  );
}

// CIRepositoryRow is one Repository's CI entry, edited in place.
function CIRepositoryRow({ project, entry }: { project: string; entry: CIRepositoryView }) {
  const api = useApi();
  const id = useId();
  const [editing, setEditing] = useState(false);
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
    api.setRepositoryCI(project, entry.name, change).then(
      () => setEditing(false),
      (e: ApiError) => setError(e),
    );
  };

  if (!editing) {
    return (
      <li>
        <strong>{entry.name}</strong>: {ciTarget(entry)}{" "}
        <Button type="button" onClick={() => setEditing(true)} aria-label={`Edit ${entry.name}'s CI`}>
          Edit
        </Button>
      </li>
    );
  }
  return (
    <li>
      <form aria-label={`${entry.name}'s CI`} onSubmit={save}>
        <strong>{entry.name}</strong>
        <label className="project-options-field">
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
          <Button type="button" onClick={() => setEditing(false)}>
            Cancel
          </Button>
        </div>
      </form>
    </li>
  );
}
