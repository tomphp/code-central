// What a Spec, Requirement or Map pane's place shows until it has a Spec to
// draw: the read's error, or Loading… while there is none yet.
export function SpecPaneNote({ error }: { error?: string }) {
  return <p className="empty">{error ?? "Loading…"}</p>;
}
