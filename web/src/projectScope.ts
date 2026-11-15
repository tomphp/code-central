// The selected Project: the one the URL names, else the Sole Project (the
// only Project that isn't Archived), else none, which is All Projects. It is
// worked out from the URL and the live Project list on every render and is
// never written into the address.
import { createContext, useContext } from "react";
import type { ProjectRow } from "./api/types";

// soleProject is the key of the only Project that isn't Archived; with none
// or several such Projects there is none.
export function soleProject(projects: Pick<ProjectRow, "key" | "archived">[]): string | undefined {
  const active = projects.filter((p) => !p.archived);
  return active.length === 1 ? active[0].key : undefined;
}

// selectedProject is the URL's Project, else the Sole Project. A named
// Project always wins, an Archived or unknown one included.
export function selectedProject(urlProject: string | undefined, projects: Pick<ProjectRow, "key" | "archived">[]): string | undefined {
  return urlProject || soleProject(projects);
}

export interface ProjectScope {
  // project is the selected Project's key, undefined for All Projects.
  project?: string;
  // sole says there is a Sole Project, so All Projects isn't offered, even
  // while the URL names another Project.
  sole: boolean;
  // resolved says the scope is known: the URL names a Project, or the
  // Project list has loaded or failed to.
  resolved: boolean;
}

// Outside the App, the scope is All Projects, as the URL alone gave it.
export const ProjectScopeContext = createContext<ProjectScope>({ sole: false, resolved: true });

export const useProjectScope = () => useContext(ProjectScopeContext);

// projectScopeOf works the scope out. projects is undefined until the list
// first loads; failed says the read failed, which resolves with no Sole Project.
export function projectScopeOf(urlProject: string | undefined, projects: ProjectRow[] | undefined, failed: boolean): ProjectScope {
  if (!projects) return { project: urlProject, sole: false, resolved: !!urlProject || failed };
  const sole = soleProject(projects);
  return { project: urlProject || sole, sole: !!sole, resolved: true };
}
