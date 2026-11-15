import { count } from "../format";

// Tokens is a token count's cell, with thousands separators and the
// exact figure, in tokens, on hover.
export function Tokens({ n }: { n: number }) {
  return (
    <td className="num">
      <TokenCount n={n} />
    </td>
  );
}

// TokenCount is a token count with thousands separators and the exact
// figure, in tokens, on hover.
export function TokenCount({ n }: { n: number }) {
  return <span title={`${count(n)} tokens`}>{count(n)}</span>;
}
