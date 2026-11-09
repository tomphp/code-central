// A Map body's sections, read from its fixed level-2 headings
// (Destination, Notes, Decisions so far, Not yet specified, Out of
// scope), as core's section reader reads them for get_spec's sections.

export type MapSection = "Destination" | "Notes" | "Decisions so far" | "Not yet specified" | "Out of scope";

// mapSection is the text under the body's `## <heading>`, up to the next
// heading of level 1 or 2, trimmed; "" when the heading is missing or
// empty. Headings inside fenced code don't count.
export function mapSection(body: string, heading: MapSection): string {
  const out: string[] = [];
  let inside = false;
  let fenced = false;
  for (const line of body.split("\n")) {
    if (/^\s*(```|~~~)/.test(line)) fenced = !fenced;
    const h = !fenced && /^#{1,2}\s+(.*?)\s*#*\s*$/.exec(line);
    if (h) {
      if (inside) break;
      inside = line.startsWith("## ") && h[1].toLowerCase() === heading.toLowerCase();
      continue;
    }
    if (inside) out.push(line);
  }
  return out.join("\n").trim();
}
