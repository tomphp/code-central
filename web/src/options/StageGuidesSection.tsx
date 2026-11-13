import {
  useEffect,
  useId,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import type { Api } from "../api/api";
import type { ApiError, ProjectRow } from "../api/types";
import { Refusal } from "./Refusal";

// StageGuidesSection is the Project's Stage Guides settings (Specs
// code-central-441 and code-central-464): the Subagent Exploration and Model
// Assignment checkboxes, each with its own Save. Like the other sections it
// follows the Project as read.
export function StageGuidesSection({ api, project }: { api: Api; project: ProjectRow }) {
  return (
    <fieldset id="project-options-stage-guides" className="project-options-section" disabled={project.archived}>
      <legend>Stage Guides</legend>
      <StageGuideCheckbox
        label="Subagent Exploration"
        stored={project.stage_guides.subagent_exploration}
        save={(on) => api.setSubagentExploration(project.key, on).then((v) => v.stage_guides.subagent_exploration)}
      >
        While on, the implementation guide tells the agent to ask an Explore
        subagent when understanding existing code means reading across several
        files, so only its conclusion stays in context. Turn it off if your
        agents have no subagents or can&apos;t afford them. A change applies
        from the next Claim and the next get_stage_guide call, not to a
        running or resumed session.
      </StageGuideCheckbox>
      <StageGuideCheckbox
        label="Model Assignment"
        stored={project.stage_guides.model_assignment}
        save={(on) => api.setModelAssignment(project.key, on).then((v) => v.stage_guides.model_assignment)}
      >
        While on, the tickets Stage gives each Ticket it creates a model and
        effort, chosen from the Project&apos;s Reported Models. A change applies
        from the next get_stage_guide call.
      </StageGuideCheckbox>
    </fieldset>
  );
}

// StageGuideCheckbox is one Stage Guides checkbox with its hint and its own
// Save; save resolves to the value the Project now stores.
function StageGuideCheckbox({
  label,
  stored,
  save: persist,
  children,
}: {
  label: string;
  stored: boolean;
  save: (on: boolean) => Promise<boolean>;
  children: ReactNode;
}) {
  const [draft, setDraft] = useState(stored);
  const [error, setError] = useState<ApiError>();
  const [saved, setSaved] = useState(false);
  const id = useId();
  useEffect(() => setDraft(stored), [stored]);

  const save = (e: FormEvent) => {
    e.preventDefault();
    setError(undefined);
    persist(draft).then(
      (on) => {
        setDraft(on);
        setSaved(true);
      },
      (e: ApiError) => setError(e),
    );
  };
  const changed = draft !== stored;

  return (
    <form onSubmit={save}>
      <label>
        <input
          type="checkbox"
          checked={draft}
          onChange={(e) => {
            setDraft(e.target.checked);
            setError(undefined);
            setSaved(false);
          }}
          aria-describedby={`${id}-hint`}
        />{" "}
        {label}
      </label>
      <p id={`${id}-hint`} className="project-options-hint">
        {children}
      </p>
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
  );
}
