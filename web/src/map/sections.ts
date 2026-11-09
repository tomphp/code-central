// A Map body's sections, read from its fixed level-2 headings
// (Destination, Notes, Decisions so far, Not yet specified, Out of
// scope) the way core's section reader (core/section.go, get_spec's
// sections) reads them: ATX headings outside fenced code, matched
// case-insensitively, each section running to the next heading at its
// level or higher.

export type MapSection = "Destination" | "Notes" | "Decisions so far" | "Not yet specified" | "Out of scope";

// As core's atx, closing and fence patterns.
const ATX = /^ {0,3}(#{1,6})(?:[ \t]+(.*?))?[ \t]*$/;
const CLOSING = /(?:^|[ \t]+)#+$/;
const FENCE = /^ {0,3}(`{3,}|~{3,})/;

// mapSection is the text under the body's level-2 heading, trimmed; ""
// when the heading is missing or empty.
export function mapSection(body: string, heading: MapSection): string {
  const out: string[] = [];
  let inside = false;
  let open = "";
  for (const raw of body.split("\n")) {
    const line = raw.replace(/\r$/, "");
    const fence = FENCE.exec(line);
    if (fence) {
      if (!open) open = fence[1];
      else if (closesFence(fence[1], line.trimStart().slice(fence[1].length), open)) open = "";
    } else if (!open) {
      const h = ATX.exec(line);
      if (h && inside && h[1].length <= 2) break;
      if (h && h[1].length === 2 && (h[2] ?? "").replace(CLOSING, "").trim().toLowerCase() === heading.toLowerCase()) {
        inside = true;
        continue;
      }
    }
    if (inside) out.push(line);
  }
  return out.join("\n").trim();
}

// closesFence reports whether a fence line closes the open fence: the same
// character, at least as many, and nothing after them, as core's.
function closesFence(marks: string, rest: string, open: string): boolean {
  return marks[0] === open[0] && marks.length >= open.length && rest.trim() === "";
}
