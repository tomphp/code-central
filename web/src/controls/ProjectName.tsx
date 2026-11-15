import type { ProjectRow } from "../api/types";

// projectName looks a Project's display name up by key, falling back to the key.
export function projectName(projects: Pick<ProjectRow, "key" | "display_name">[], key: string): string {
  return projects.find((p) => p.key === key)?.display_name ?? key;
}

// ProjectName shows a Project's display name with its key, muted.
export function ProjectName({ name, projectKey }: { name: string; projectKey: string }) {
  return (
    <>
      {name} <span className="muted">{projectKey}</span>
    </>
  );
}
