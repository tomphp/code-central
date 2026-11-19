// A Spec's Tags on the plain Spec and Requirement panes (Spec
// code-central-1089): each Tag with its value to edit or remove, and a
// form that adds one. Core checks every write, such as a `branch` Tag's
// name, its clash with another Spec and its lock, and its refusal is
// shown beside the Tag it refused.
import { useState, type FormEvent } from "react";
import { projectOf } from "../api/api";
import type { SpecDetail } from "../api/types";
import { Button } from "../controls/Button";
import { ErrorBanner } from "../controls/ErrorBanner";
import { useApi } from "../live";
import { useAct } from "../ticket/useAct";

export interface SpecTagsProps {
  spec: SpecDetail;
  // archived disables every write, as the Project's being Archived does.
  archived?: boolean;
}

export function SpecTags({ spec, archived }: SpecTagsProps) {
  const entries = Object.entries(spec.tags).sort(([a], [b]) => a.localeCompare(b));
  return (
    <section aria-label="Tags" className="spec-tags">
      <h3 className="spec-tags-head">Tags</h3>
      {entries.length === 0 ? (
        <p className="muted small">No Tags.</p>
      ) : (
        <ul className="spec-tags-list">
          {/* Keyed on the value too, so the draft follows a change made elsewhere. */}
          {entries.map(([key, value]) => (
            <TagRow key={`${key}=${value}`} id={spec.id} tagKey={key} value={value} archived={archived} />
          ))}
        </ul>
      )}
      <AddTag id={spec.id} archived={archived} />
    </section>
  );
}

// TagRow edits or removes the Tag tagKey of the Spec id.
function TagRow({ id, tagKey, value, archived }: { id: string; tagKey: string; value: string; archived?: boolean }) {
  const api = useApi();
  const { busy, error, run } = useAct();
  const [draft, setDraft] = useState(value);
  const next = draft.trim();
  const save = (e: FormEvent) => {
    e.preventDefault();
    void run(() => api.setTags(projectOf(id), id, { [tagKey]: next }));
  };
  const remove = () => void run(() => api.removeTags(projectOf(id), id, [tagKey]));
  return (
    <li aria-label={tagKey}>
      <form className="spec-tag" onSubmit={save}>
        <code className="spec-tag-key">{tagKey}</code>
        <input aria-label={`${tagKey} Tag value`} value={draft} disabled={archived} onChange={(e) => setDraft(e.target.value)} />
        <Button type="submit" aria-label={`Save ${tagKey}`} disabled={archived || busy || !next || next === value}>
          Save
        </Button>
        <Button type="button" aria-label={`Remove ${tagKey}`} disabled={archived || busy} onClick={remove}>
          Remove
        </Button>
      </form>
      {error && <ErrorBanner>{error}</ErrorBanner>}
    </li>
  );
}

// AddTag adds a Tag to the Spec id, or sets it if the Spec has one of that key.
function AddTag({ id, archived }: { id: string; archived?: boolean }) {
  const api = useApi();
  const { busy, error, run } = useAct();
  const [key, setKey] = useState("");
  const [value, setValue] = useState("");
  const tag = { key: key.trim(), value: value.trim() };
  const add = async (e: FormEvent) => {
    e.preventDefault();
    // A refusal keeps the draft, to correct it.
    if (await run(() => api.setTags(projectOf(id), id, { [tag.key]: tag.value }))) {
      setKey("");
      setValue("");
    }
  };
  return (
    <form aria-label="Add a Tag" className="spec-tag-add" onSubmit={add}>
      <div className="spec-tag">
        <input aria-label="Tag key" placeholder="key" value={key} disabled={archived} onChange={(e) => setKey(e.target.value)} />
        <input aria-label="Tag value" placeholder="value" value={value} disabled={archived} onChange={(e) => setValue(e.target.value)} />
        <Button type="submit" disabled={archived || busy || !tag.key || !tag.value}>
          Add
        </Button>
      </div>
      {error && <ErrorBanner>{error}</ErrorBanner>}
    </form>
  );
}
