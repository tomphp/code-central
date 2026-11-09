// "New requirement", "New Map" and "New Prototype": buttons that each
// open the same modal, one free-text box. A new Requirement or Map goes to
// the filtered Project, or the only one; with several and none filtered,
// the modal asks which. A Prototype goes on the Spec whose view it is on.
// "Write the Spec", in a finished Map's header, opens the New requirement
// modal prefilled from the Map and submits it with the Map.
import { useEffect, useId, useState, type FormEvent } from "react";
import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import type { ApiError, ProjectRow, SpecAcknowledgement, TicketAcknowledgement } from "../api/types";

export interface NewRequirementProps {
  api: Api;
  // projects are the Projects a Requirement or Map can go to: the active ones.
  projects: ProjectRow[];
  // project is the filtered Project, if any.
  project?: string;
  onSubmitted(r: { spec: SpecAcknowledgement }): void;
}

// Kind is what a button submits: its words, and the write it calls,
// which resolves to R.
interface Kind<R> {
  button: string;
  className: string;
  // noun names the text box.
  noun: string;
  placeholder: string;
  creates: string;
  // text prefills the text box.
  text?: string;
  submit(api: Api, project: string, text: string): Promise<R>;
}

const REQUIREMENT: Kind<{ spec: SpecAcknowledgement }> = {
  button: "New requirement",
  className: "btn primary",
  noun: "Requirement",
  placeholder: "Describe what you want built. The first line becomes the working title.",
  creates: "Creates a Spec and its Grilling, Spec and Tickets Stages. A Factory picks up Grilling.",
  submit: (api, project, text) => api.submitRequirement(project, text),
};

const MAP: Kind<{ spec: SpecAcknowledgement }> = {
  button: "New Map",
  className: "btn",
  noun: "Map",
  placeholder: "Describe where you want to get to. The first line becomes the working title.",
  creates: "Creates a Map and its Charting Ticket. A Factory picks up Charting.",
  submit: (api, project, text) => api.submitMap(project, text),
};

// prototypeOn is New Prototype on spec.
const prototypeOn = (spec: string): Kind<TicketAcknowledgement> => ({
  button: "New Prototype",
  className: "btn",
  noun: "Prototype",
  placeholder: "Describe what to try out, and the decision it should inform. The first line becomes the title.",
  creates: "Creates a Ready Prototype on this Spec. A Factory builds it on a branch that never lands.",
  submit: (api, project, text) => api.submitPrototype(project, spec, text),
});

export function NewRequirement(props: NewRequirementProps) {
  return <NewFromText kind={REQUIREMENT} {...props} />;
}

export function NewMap(props: NewRequirementProps) {
  return <NewFromText kind={MAP} {...props} />;
}

export interface WriteTheSpecProps {
  api: Api;
  // map is the finished Map, with its Destination.
  map: { id: string; title: string; destination: string };
  onSubmitted(r: { spec: SpecAcknowledgement }): void;
}

// WriteTheSpec makes a Requirement from a finished Map, with no grilling
// Stage: its text starts as the Map's title, a link to it and its
// Destination.
export function WriteTheSpec({ api, map, onSubmitted }: WriteTheSpecProps) {
  const kind: Kind<{ spec: SpecAcknowledgement }> = {
    ...REQUIREMENT,
    button: "Write the Spec",
    className: "btn",
    creates: "Creates a Spec and its Spec and Tickets Stages, from the Map's decisions. A Factory picks up Spec.",
    text: [map.title, `From the Map [${map.id}](${map.id}).`, map.destination].filter(Boolean).join("\n\n") + "\n",
    submit: (api, project, text) => api.submitRequirement(project, text, map.id),
  };
  return <NewFromText kind={kind} api={api} projects={[]} project={projectOf(map.id)} onSubmitted={onSubmitted} />;
}

export interface NewPrototypeProps {
  api: Api;
  // spec is the Spec the Prototype goes on.
  spec: string;
  onSubmitted(t: TicketAcknowledgement): void;
}

export function NewPrototype({ api, spec, onSubmitted }: NewPrototypeProps) {
  return <NewFromText kind={prototypeOn(spec)} api={api} projects={[]} project={projectOf(spec)} onSubmitted={onSubmitted} />;
}

interface NewFromTextProps<R> {
  kind: Kind<R>;
  api: Api;
  projects: ProjectRow[];
  project?: string;
  onSubmitted(r: R): void;
}

function NewFromText<R>({ kind, api, projects, project, onSubmitted }: NewFromTextProps<R>) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button className={kind.className} onClick={() => setOpen(true)}>
        {kind.button}
      </button>
      {open && (
        <Modal
          kind={kind}
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

function Modal<R>({ kind, api, projects, project, onClose, onSubmitted }: NewFromTextProps<R> & { onClose(): void }) {
  const [text, setText] = useState(kind.text ?? "");
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
    kind.submit(api, chosen, text).then(onSubmitted, (e: ApiError) => {
      setBusy(false);
      setError(e.message);
    });
  };

  return (
    <div className="modal" onClick={(e) => e.target === e.currentTarget && onClose()}>
      <div className="modal-card" role="dialog" aria-modal="true" aria-labelledby={titleId}>
        <h2 id={titleId}>{kind.button}</h2>
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
            aria-label={kind.noun}
            placeholder={kind.placeholder}
            rows={8}
            value={text}
            autoFocus
            onChange={(e) => setText(e.target.value)}
          />
          {error && <p className="warning">{error}</p>}
          <div className="modal-actions">
            <span className="muted small">{kind.creates}</span>
            <button type="button" className="btn" onClick={onClose}>
              Cancel
            </button>
            <button type="submit" className="btn primary" disabled={busy || !text.trim() || !chosen}>
              Submit {kind.noun.toLowerCase()}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
