// PartlyLandedBadge marks an Accepted Ticket that has landed in some of
// its Changed Repositories but not all (Spec code-central-848): a status,
// not a State, like Waiting for the Spec. It renders nothing otherwise.
export function PartlyLandedBadge({ partly }: { partly?: boolean }) {
  if (!partly) return null;
  return <span className="label partly-landed-badge">Partly landed</span>;
}
