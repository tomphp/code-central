// The CI status badge (code-central-259): a Ticket's CI result on its rows
// in the Spec view, Needs Me and the Activity Dashboard, and the Spec
// view's sums of them, so CI health scans at a glance.
import type { CIResult } from "../api/types";

// CI_RESULTS is the order the sums go in: what needs the human first.
const CI_RESULTS: CIResult[] = ["failed", "running", "passed"];

// CIBadge is a row's badge; it shows nothing for a Ticket with no CI
// result.
export function CIBadge({ result }: { result?: CIResult }) {
  if (!result) return null;
  return <span className={`ci-result ci-badge ci-${result}`}>CI {result}</span>;
}

// CISums counts tickets' badges per result, leaving out a result none of
// them has; it shows nothing when none has a CI result.
export function CISums({ tickets }: { tickets: { ci_result?: CIResult }[] }) {
  const counts = CI_RESULTS.map((result) => ({ result, n: tickets.filter((t) => t.ci_result === result).length })).filter((c) => c.n > 0);
  if (counts.length === 0) return null;
  return (
    <p role="group" aria-label="CI results" className="ci-sums">
      <span className="muted small">CI</span>
      {counts.map(({ result, n }) => (
        <span key={result} className={`ci-result ci-${result}`}>
          {n} {result}
        </span>
      ))}
    </p>
  );
}
