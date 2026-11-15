// Count is the one count: a muted number in tabular figures, or the loud
// one when something in it needs the human. of names what it counts, after
// the number ("2 left"). big is a count block's number, in big type in the
// block's own colour; the tests find it by its test ID. n is undefined
// until the count is known, and shows as a dash.
export interface CountProps {
  n?: number;
  loud?: boolean;
  of?: string;
  big?: boolean;
}

export function Count({ n, loud, of, big }: CountProps) {
  const shown = n === undefined ? "–" : n;
  return (
    <span className={big ? "count big" : loud ? "count loud" : "count"} data-testid={big ? "count" : undefined}>
      {of ? `${shown} ${of}` : shown}
    </span>
  );
}
