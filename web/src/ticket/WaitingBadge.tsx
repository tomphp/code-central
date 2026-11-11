// The Waiting for the Spec badge (code-central-371): an Accepted Ticket
// whose work is gathered on its Spec Branch shows it beside the State,
// until its Spec lands.
export function WaitingBadge({ waiting }: { waiting?: boolean }) {
  if (!waiting) return null;
  return <span className="label waiting-badge">Waiting for the Spec</span>;
}
