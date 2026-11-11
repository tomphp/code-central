// The edit form every Edit opens: a title input and a Markdown textarea,
// with Preview, Save and Cancel. Its draft is its own state, so a live
// update to the record doesn't touch it; Save sends only the fields that
// differ from what the form opened with, and the last write wins.
import { useState } from "react";
import { Markdown } from "../Markdown";
import { useAct } from "../ticket/useAct";

export interface EditFormProps {
  title: string;
  text: string;
  // label names the text: "description" or "body".
  label: "description" | "body";
  // save writes the fields that changed; a refusal rejects, and its
  // message is shown with the form left open.
  save(change: { title?: string; text?: string }): Promise<unknown>;
  close(): void;
}

export function EditForm({ title, text, label, save, close }: EditFormProps) {
  const [draftTitle, setTitle] = useState(title);
  const [draftText, setText] = useState(text);
  const [preview, setPreview] = useState(false);
  const { busy, error, run } = useAct();
  const name = label[0].toUpperCase() + label.slice(1);
  return (
    <form
      className="edit-form"
      onSubmit={(e) => {
        e.preventDefault();
        const change = {
          ...(draftTitle !== title && { title: draftTitle }),
          ...(draftText !== text && { text: draftText }),
        };
        if (!Object.keys(change).length) return close();
        void run(async () => {
          await save(change);
          close();
        });
      }}
    >
      <input aria-label="Title" value={draftTitle} onChange={(e) => setTitle(e.target.value)} />
      {preview ? (
        <div className="edit-preview" aria-label={`${name} preview`}>
          {draftText ? <Markdown>{draftText}</Markdown> : <p className="muted">No {label}.</p>}
        </div>
      ) : (
        <textarea aria-label={name} rows={12} value={draftText} onChange={(e) => setText(e.target.value)} />
      )}
      <div className="edit-actions">
        <button className="btn primary" disabled={busy}>
          Save
        </button>{" "}
        <button type="button" className="btn" onClick={close}>
          Cancel
        </button>{" "}
        <button type="button" className="btn" aria-pressed={preview} onClick={() => setPreview(!preview)}>
          Preview
        </button>
      </div>
      {error && (
        <p role="alert" className="warning">
          {error}
        </p>
      )}
    </form>
  );
}
