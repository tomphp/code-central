import type { LandPerSpecStatus } from "../api/types";

// LandPerSpec is a Spec's Land per Spec status (code-central-371):
// how many Accepted Tickets wait for the Spec to land, and "merging" once
// it is ready and the Factory merges it into the base.
export function LandPerSpec({ status }: { status?: LandPerSpecStatus }) {
  if (!status || status.waiting.length === 0) return null;
  const n = status.waiting.length;
  return (
    <p role="status" aria-label="Land per Spec" className="land-per-spec small">
      {status.ready ? "Merging into the base" : `${n} ${n === 1 ? "Ticket" : "Tickets"} waiting for the Spec`}
      {status.ready && ` · ${n} ${n === 1 ? "Ticket" : "Tickets"} land with it`}
    </p>
  );
}
