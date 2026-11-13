// The triage actions (Spec code-central-279): what the human may do with
// an Incoming Issue, offered on every one, awaiting triage or not: Keep
// as Draft, To Requirement with its context box, and Won't fix with its
// reason box.
import { useState } from "react";
import type { Api } from "../api/api";
import "../api/actions/keep_as_draft";
import "../api/actions/to_requirement";
import "../api/actions/wont_fix";
import type { TicketView } from "../api/types";
import { useAct } from "./useAct";
import { Button } from "../controls/Button";

// BOXES are the triage actions that open a box before confirming: the
// box's label, the field its text is sent as, and what it says.
const BOXES = {
  to_requirement: {
    title: "To Requirement",
    field: "context",
    label: "Context (optional), added to the Requirement and posted to the issue as a comment",
    rows: 3,
    note: "To Requirement makes a Requirement of the issue's title and body, Cancels this Ticket, and from then on only writes the work's progress to the issue.",
  },
  wont_fix: {
    title: "Won't fix",
    field: "reason",
    label: "Reason (optional), posted to the issue as a comment",
    rows: 2,
    note: "Won't fix Cancels this Ticket and closes the issue on GitHub as not planned.",
  },
} as const;

type BoxAction = keyof typeof BOXES;

export function TriageActions({ api, project, ticket: t }: { api: Api; project: string; ticket: TicketView }) {
  const { busy, error, run } = useAct();
  // open is the triage action whose box is open, if any.
  const [open, setOpen] = useState<BoxAction>();
  const [text, setText] = useState("");
  const actions = t.triage_actions ?? [];
  const box = open && BOXES[open];
  return (
    <>
      <p className="actions">
        {actions.includes("keep_as_draft") && (
          <Button type="button"  disabled={busy} onClick={() => run(() => api.act(project, t.id, "keep_as_draft", {}))}>
            Keep as Draft
          </Button>
        )}
        {!open &&
          (Object.keys(BOXES) as BoxAction[])
            .filter((a) => actions.includes(a))
            .map((a) => (
              <Button
                key={a}
                type="button"

                disabled={busy}
                onClick={() => {
                  setText("");
                  setOpen(a);
                }}
              >
                {BOXES[a].title}…
              </Button>
            ))}
      </p>
      {open && box && (
        <form
          aria-label={box.title}
          onSubmit={(e) => {
            e.preventDefault();
            const v = text.trim();
            run(() => api.act(project, t.id, open, v ? { [box.field]: v } : {}));
          }}
        >
          <label>
            <span className="small muted">{box.label}</span>
            <textarea rows={box.rows} value={text} onChange={(e) => setText(e.target.value)} />
          </label>
          <p className="small muted">{box.note}</p>
          <p className="actions">
            <Button type="submit" variant="primary" disabled={busy}>
              {box.title}
            </Button>
            <Button type="button"  disabled={busy} onClick={() => setOpen(undefined)}>
              Back
            </Button>
          </p>
        </form>
      )}
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
    </>
  );
}

// Triage is an Incoming Issue's triage actions in its Ticket details,
// where its action panel doesn't already offer them.
export function Triage(props: { api: Api; project: string; ticket: TicketView }) {
  return (
    <section aria-label="Triage">
      <p className="muted small">
        An Incoming Issue from GitHub Issues. Keep as Draft leaves it here as it is{props.ticket.awaiting_triage && ", out of Needs Me"}, and
        writes nothing to GitHub. To Requirement makes a Requirement of it. Won't fix Cancels it and closes the issue as not planned.
      </p>
      <TriageActions {...props} />
    </section>
  );
}
