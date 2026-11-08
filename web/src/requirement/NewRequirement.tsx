// "New requirement": the left pane's primary button and its modal, one
// free-text box. It submits to the filtered Project, or the only one;
// with several and none filtered, the modal asks which.
import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import type { ApiError, ProjectRow, SubmitRequirementResult } from "../api/types";

export interface NewRequirementProps {
  api: Api;
  // projects are the Projects a Requirement can go to: the active ones.
  projects: ProjectRow[];
  // project is the filtered Project, if any.
  project?: string;
  onSubmitted(r: SubmitRequirementResult): void;
}

export function NewRequirement({ api, projects, project, onSubmitted }: NewRequirementProps) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button className="btn primary new-req" onClick={() => setOpen(true)}>
        New requirement
      </button>
      {open && (
        <Modal
          api={api}
          projects={projects}
          project={project}
          onClose={() => setOpen(false)}
          onSubmitted={(r) => {
            setOpen(false);
            onSubmitted(r);
          }}
        />
      )}
    </>
  );
}

function Modal({ api, projects, project, onClose, onSubmitted }: NewRequirementProps & { onClose(): void }) {
  const [text, setText] = useState("");
  const [chosen, setChosen] = useState(project ?? projects[0]?.key ?? "");
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const titleId = useId();
  const ask = !project && projects.length > 1;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [onClose]);

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!text.trim() || !chosen) return;
    setBusy(true);
    setError(undefined);
    api.submitRequirement(chosen, text).then(onSubmitted, (e: ApiError) => {
      setBusy(false);
      setError(e.message);
    });
  };

  return (
    <div className="modal" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <h2 id={titleId}>New requirement</h2>
        <form onSubmit={submit}>
          {ask && (
            <label className="modal-project small">
              Project{" "}
              <select value={chosen} onChange={(e) => setChosen(e.target.value)}>
                {projects.map((p) => (
                  <option key={p.key} value={p.key}>
                    {p.display_name}
                  </option>
                ))}
              </select>
            </label>
          )}
          <textarea
            aria-label="Requirement"
            placeholder="Describe what you want built. The first line becomes the working title."
            rows={8}
            value={text}
            autoFocus
            onChange={(e) => setText(e.target.value)}
          />
          {error && <p className="warning">{error}</p>}
          <div className="modal-actions">
            <span className="muted small">Creates a Spec and its Grilling, Spec and Tickets Stages. A Factory picks up Grilling.</span>
            <button type="button" className="btn" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="btn primary" disabled={busy || !text.trim() || !chosen}>
              Submit requirement
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
