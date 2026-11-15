// KindsTable is Usage's Context Size by kind of session table: how many
// sessions there were, and the average and largest of their peaks.
import type { UsageKind } from "../api/types";
import { count } from "../format";
import { Tokens } from "./TokenCount";

export function KindsTable({ kinds }: { kinds: UsageKind[] }) {
  return (
    <table aria-label="Context Size by kind" className="data-table data-table-mono">
      <thead>
        <tr>
          <th scope="col">Kind</th>
          <th scope="col" className="num">Sessions</th>
          <th scope="col" className="num">Average peak</th>
          <th scope="col" className="num">Largest peak</th>
        </tr>
      </thead>
      <tbody>
        {kinds.map((k) => (
          <tr key={k.kind}>
            <th scope="row">{k.kind}</th>
            <td className="num">{count(k.sessions)}</td>
            <Tokens n={k.average_peak} />
            <Tokens n={k.largest_peak} />
          </tr>
        ))}
      </tbody>
    </table>
  );
}
