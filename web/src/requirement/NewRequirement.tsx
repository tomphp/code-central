// "New requirement", "New Map", "New Small Change" and "New Prototype": buttons that each
// open the same modal, one free-text box. A new Requirement or Map goes to
// the filtered Project, or the only one; with several and none filtered,
// the modal asks which. A Prototype goes on the Spec whose view it is on.
// "Write the Spec", in a finished Map's header, opens the New requirement
// modal prefilled from the Map and submits it with the Map. A caller can
// render its own opening control in place of the button, such as a menu
// item; the modal goes in a portal on the body, so it outlives a menu that
// closes behind it.
import { useId, useState, type FormEvent, type ReactNode } from "react";
import type { Api } from "../api/api";
import { projectOf } from "../api/api";
import type { ApiError, ProjectRow, SpecAcknowledgement, TicketAcknowledgement } from "../api/types";
import { Button } from "../controls/Button";
import { Modal as Shell } from "../controls/Modal";
import { Select } from "../controls/Select";

export interface NewRequirementProps {
  api: Api;
  // projects are the Projects a Requirement, Map or Small Change can go to:
  // the active ones.
  projects: ProjectRow[];
  // project is the filtered Project, if any.
  project?: string;
  onSubmitted(r: { spec: SpecAcknowledgement }): void;
  // trigger renders the control that opens the modal, given what opens
  // it, in place of the button.
  trigger?(open: () => void): ReactNode;
}

// Kind is what a button submits: its words, and the write it calls,
// which resolves to R.
interface Kind<R> {
  button: string;
  variant: "default" | "primary";
  // noun names the text box.
  noun: string;
  placeholder: string;
  creates: string;
  // text prefills the text box.
  text?: string;
  // straight offers the "Straight to Spec" checkbox, and says what ticking
  // it creates.
  straight?: { creates: string };
  // draftable shows Save as Draft, which calls submit with draft true.
  draftable?: boolean;
  submit(api: Api, project: string, text: string, draft: boolean, straight: boolean): Promise<R>;
}

const REQUIREMENT: Kind<{ spec: SpecAcknowledgement }> = {
  button: "New requirement",
  variant: "primary",
  noun: "Requirement",
  placeholder: "Describe what you want built. The first line becomes the working title.",
  creates: "Creates a Spec and its Grilling, Spec and Tickets Stages. A Factory picks up Grilling.",
  straight: { creates: "Creates a Spec and its Spec and Tickets Stages. A Factory picks up Spec." },
  draftable: true,
  submit: (api, project, text, draft, straight) => api.submitRequirement(project, text, undefined, straight, draft),
};

const MAP: Kind<{ spec: SpecAcknowledgement }> = {
  button: "New Map",
  variant: "default",
  noun: "Map",
  placeholder: "Describe where you want to get to. The first line becomes the working title.",
  creates: "Creates a Map and its Charting Ticket. A Factory picks up Charting.",
  draftable: true,
  submit: (api, project, text, draft) => api.submitMap(project, text, draft),
};

const SMALL_CHANGE: Kind<TicketAcknowledgement> = {
  button: "New Small Change",
  variant: "default",
  noun: "Small Change",
  placeholder: "Describe the change you want. The first line becomes the Ticket's title.",
  creates: "Creates a Ready Ticket on the Small Changes Spec. A Factory builds it and it lands on its own.",
  draftable: true,
  submit: (api, project, text, draft) => api.submitSmallChange(project, text, draft),
};

// prototypeOn is New Prototype on spec.
const prototypeOn = (spec: string): Kind<TicketAcknowledgement> => ({
  button: "New Prototype",
  variant: "default",
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

export interface NewSmallChangeProps extends Omit<NewRequirementProps, "onSubmitted"> {
  onSubmitted(t: TicketAcknowledgement): void;
}

export function NewSmallChange(props: NewSmallChangeProps) {
  return <NewFromText kind={SMALL_CHANGE} {...props} />;
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
    straight: undefined,
    draftable: false,
    button: "Write the Spec",
    variant: "default",
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
  trigger?(open: () => void): ReactNode;
}

function NewFromText<R>({ kind, api, projects, project, onSubmitted, trigger }: NewFromTextProps<R>) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {trigger ? (
        trigger(() => setOpen(true))
      ) : (
        <Button variant={kind.variant} onClick={() => setOpen(true)}>
          {kind.button}
        </Button>
      )}
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
  // Until the human picks one, the Project follows the props, so a modal
  // opened before the Projects are read still has one once they are.
  const [picked, setPicked] = useState<string>();
  const chosen = picked ?? project ?? projects[0]?.key ?? "";
  const [error, setError] = useState<string>();
  const [busy, setBusy] = useState(false);
  const [straight, setStraight] = useState(false);
  const titleId = useId();
  const ask = !project && projects.length > 1;

  const send = (draft: boolean) => {
    if (!text.trim() || !chosen) return;
    setBusy(true);
    setError(undefined);
    kind.submit(api, chosen, text, draft, straight).then(onSubmitted, (e: ApiError) => {
      setBusy(false);
      setError(e.message);
    });
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    send(false);
  };

  return (
    <Shell labelledBy={titleId} onClose={onClose}>
        <h2 id={titleId}>{kind.button}</h2>
        <form onSubmit={submit}>
          {ask && (
            <label className="modal-project small">
              Project{" "}
              <Select
                value={chosen}
                onChange={setPicked}
                options={projects.map((p) => ({ value: p.key, label: p.display_name }))}
              />
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
          {kind.straight && (
            <label className="small">
              <input type="checkbox" checked={straight} onChange={(e) => setStraight(e.target.checked)} /> Straight to Spec (no grilling)
            </label>
          )}
          {error && <p className="warning">{error}</p>}
          <div className="modal-actions">
            <span className="muted small">{kind.straight && straight ? kind.straight.creates : kind.creates}</span>
            <Button type="button"  onClick={onClose}>
              Cancel
            </Button>
            {kind.draftable && (
              <Button type="button" disabled={busy || !text.trim() || !chosen} onClick={() => send(true)}>
                Save as Draft
              </Button>
            )}
            <Button type="submit" variant="primary" disabled={busy || !text.trim() || !chosen}>
              Submit {kind.noun.toLowerCase()}
            </Button>
          </div>
        </form>
    </Shell>
  );
}
