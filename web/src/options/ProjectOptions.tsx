// The Project Options page: one Project's settings, reached from its menu
// in the left pane, a section each: the Landing Method, CI with its CI
// Rules, GitHub Issues, Models, Stage Guides, and Focus.
import type { MouseEvent } from "react";
import type { Api } from "../api/api";
import type { ProjectRow } from "../api/types";
import { clickTo, hrefFor, type UiLocation } from "../url";
import { CISection } from "./CISection";
import { FactoriesSection } from "./FactoriesSection";
import { GitHubIssuesSection } from "./GitHubIssuesSection";
import { LandingMethodSection } from "./LandingMethodSection";
import { ModelsSection } from "./ModelsSection";
import { StageGuidesSection } from "./StageGuidesSection";
import { AutoAcceptSection } from "./AutoAcceptSection";
import { FocusSection } from "./FocusSection";

export interface ProjectOptionsProps {
  api: Api;
  now: Date;
  // projects is every Project, once read; the page shows the one keyed project.
  projects?: ProjectRow[];
  project?: string;
  navigate(patch: Partial<UiLocation>): void;
}

// SECTIONS is the one list the section menu is built from; each id is the id
// of that section's fieldset.
export const SECTIONS = [
  { id: "project-options-landing-method", label: "Landing Method" },
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
  api,
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
              <LandingMethodSection api={api} project={project} />
              <CISection api={api} project={project} />
              <GitHubIssuesSection api={api} project={project} />
              <ModelsSection api={api} project={project} />
              <StageGuidesSection api={api} project={project} />
              <AutoAcceptSection api={api} project={project} />
              <FocusSection api={api} project={project} />
              <FactoriesSection project={project} now={now} />
            </div>
          </div>
        </>
      )}
    </main>
  );
}
