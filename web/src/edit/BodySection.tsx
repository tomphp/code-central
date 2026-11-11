// A Spec's body in a collapsible section, closed by default, with Edit in
// its header while the Spec can be edited. Editing changes the Spec's
// title and body together.
import { useState } from "react";
import type { Api } from "../api/api";
import type { SpecDetail } from "../api/types";
import { Markdown } from "../Markdown";
import { EditForm } from "./EditForm";
import { specEditable } from "./editable";

export interface BodySectionProps {
  api: Api;
  spec: SpecDetail;
  heading: string;
  archived?: boolean;
}

export function BodySection({ api, spec, heading, archived = false }: BodySectionProps) {
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const project = spec.id.slice(0, spec.id.lastIndexOf("-"));
  return (
    <section aria-label={heading} className="spec-body">
      <div className="spec-body-head">
        <button type="button" className="link" aria-expanded={open || editing} disabled={editing} onClick={() => setOpen(!open)}>
          {heading}
        </button>
        {!editing && specEditable(spec, archived) && (
          <button
            type="button"
            className="btn"
            onClick={() => {
              setOpen(true);
              setEditing(true);
            }}
          >
            Edit
          </button>
        )}
      </div>
      {editing ? (
        <EditForm
          title={spec.title}
          text={spec.body}
          label="body"
          save={({ title, text }) => api.editSpec(project, spec.id, { title, body: text })}
          close={() => setEditing(false)}
        />
      ) : (
        open && (spec.body ? <Markdown>{spec.body}</Markdown> : <p className="muted">No body.</p>)
      )}
    </section>
  );
}
