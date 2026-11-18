import { ShortId } from "../inbox/ShortId";
import { ago } from "../format";

// SpecPaneHeader opens a Spec, Requirement or Map pane: its Short ID, its
// Kind and when it was submitted, and its title.
export function SpecPaneHeader({ id, kind, title, now, created, archived }: { id: string; kind: string; title: string; now: Date; created: string; archived?: boolean }) {
  return (
    <>
      <ShortId id={id} />{" "}
      {archived && (
        <>
          <span className="stage-badge">Archived</span>{" "}
        </>
      )}
      <span className="muted small">{kind} · submitted {ago(now, created)}</span>
      <h2>{title}</h2>
    </>
  );
}
