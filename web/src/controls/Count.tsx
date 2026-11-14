// Count is the one inline count: a muted number, or the loud one when
// something in it needs the human.
export function Count({ n, loud }: { n: number; loud?: boolean }) {
  return <span className={loud ? "count loud" : "count"}>{n}</span>;
}
