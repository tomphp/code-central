// The Project Options page: one Project's settings, reached from its menu
// in the left pane, a section each: the Repositories, the Default Landing,
// CI with its CI Rules, GitHub Issues, Models, Stage Guides, and Focus.
import type { MouseEvent } from "react";
import type { ProjectRow } from "../api/types";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { CISection } from "./CISection";
import { FactoriesSection } from "./FactoriesSection";
import { GitHubIssuesSection } from "./GitHubIssuesSection";
import { DefaultLandingSection } from "./DefaultLandingSection";
import { ModelsSection } from "./ModelsSection";
import { RepositoriesSection } from "./RepositoriesSection";
import { StageGuidesSection } from "./StageGuidesSection";
import { AutoAcceptSection } from "./AutoAcceptSection";
import { FocusSection } from "./FocusSection";

export interface ProjectOptionsProps {
  now: Date;
  // projects is every Project, once read; the page shows the one keyed project.
  projects?: ProjectRow[];
  project?: string;
  navigate(patch: Partial<UiLocation>): void;
}

// SECTIONS is the one list the section menu is built from; each id is the id
// of that section's fieldset.
export const SECTIONS = [
  { id: "project-options-repositories", label: "Repositories" },
  { id: "project-options-default-landing", label: "Default Landing" },
  { id: "project-options-ci", label: "CI" },
  { id: "project-options-github-issues", label: "GitHub Issues" },
  { id: "project-options-models", label: "Models" },
  { id: "project-options-stage-guides", label: "Stage Guides" },
  { id: "project-options-auto-accept", label: "Auto-accept" },
  { id: "project-options-focus", label: "Focus" },
];

// jumpTo scrolls to a section without changing the URL, which holds the view.
function jumpTo(id: string) {
  return (e: MouseEvent<HTMLAnchorElement>) => {
    e.preventDefault();
    document.getElementById(id)?.scrollIntoView?.();
  };
}

export function ProjectOptions({
  now,
  projects,
  project: key,
  navigate,
}: ProjectOptionsProps) {
  const project = projects?.find((p) => p.key === key);
  const back: UiLocation = { view: "inbox", project: project?.key };

  return (
    <main aria-label="Project Options" className="project-options">
      <a
        className="project-options-back"
        href={hrefFor(back)}
        onClick={clickTo(navigate, back)}
      >
        Back to the inbox
      </a>
      {!projects ? (
        <p className="empty">Loading…</p>
      ) : !project ? (
        <p className="empty">There is no Project {key}.</p>
      ) : (
        <>
          <h1>{project.display_name}: Project Options</h1>
          <div className="project-options-layout">
            <nav aria-label="Sections" className="project-options-menu">
              <ul>
                {SECTIONS.map((s) => (
                  <li key={s.id}>
                    <a href={`#${s.id}`} onClick={jumpTo(s.id)}>
                      {s.label}
                    </a>
                  </li>
                ))}
              </ul>
            </nav>
            <div className="project-options-sections">
              <RepositoriesSection project={project} />
              <DefaultLandingSection project={project} />
              <CISection project={project} />
              <GitHubIssuesSection project={project} />
              <ModelsSection project={project} />
              <StageGuidesSection project={project} />
              <AutoAcceptSection project={project} />
              <FocusSection project={project} />
              <FactoriesSection project={project} now={now} />
            </div>
          </div>
        </>
      )}
    </main>
  );
}
