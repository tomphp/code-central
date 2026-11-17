// The Project Options page: one Project's settings, reached from its menu
// in the left pane, a section each: the Repositories, the Landing Method,
// CI with its CI Rules, GitHub Issues, Azure Boards, Models, Stage Guides, and Focus.
import type { MouseEvent } from "react";
import type { ProjectRow } from "../api/types";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { ViewHeader } from "../controls/ViewHeader";
import { CISection } from "./CISection";
import { FactoriesSection } from "./FactoriesSection";
import { AzureBoardsSection } from "./AzureBoardsSection";
import { GitHubIssuesSection } from "./GitHubIssuesSection";
import { LandingMethodSection } from "./LandingMethodSection";
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
  { id: "project-options-landing-method", label: "Landing Method" },
  { id: "project-options-ci", label: "CI" },
  { id: "project-options-github-issues", label: "GitHub Issues" },
  { id: "project-options-azure-boards", label: "Azure Boards" },
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
    <main aria-label="Project Options" className="view-shell">
      <ViewHeader
        title={project ? `${project.display_name}: Project Options` : "Project Options"}
        right={
          <a className="btn" href={hrefFor(back)} onClick={clickTo(navigate, back)}>
            Back to the inbox
          </a>
        }
      />
      <div className="project-options">
      {!projects ? (
        <p className="empty">Loading…</p>
      ) : !project ? (
        <p className="empty">There is no Project {key}.</p>
      ) : (
        <>
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
              <LandingMethodSection project={project} />
              <CISection project={project} />
              <GitHubIssuesSection project={project} />
              <AzureBoardsSection project={project} />
              <ModelsSection project={project} />
              <StageGuidesSection project={project} />
              <AutoAcceptSection project={project} />
              <FocusSection project={project} />
              <FactoriesSection project={project} now={now} />
            </div>
          </div>
        </>
      )}
      </div>
    </main>
  );
}
