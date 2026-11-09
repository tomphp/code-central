// mapSection reads a Map body's sections as core's section reader does.
import { describe, expect, it } from "vitest";
import { mapSection } from "./sections";

describe("mapSection", () => {
  it("reads a level-2 section up to the next heading at its level or higher, subsections included", () => {
    const body = "Title\n\n## Destination\n\nOffline.\n\n### Why\n\nPlanes.\n\n## Notes\n\nNone.";
    expect(mapSection(body, "Destination")).toBe("Offline.\n\n### Why\n\nPlanes.");
  });

  it("matches headings as core does: indented, tab-separated, closed by #s and in any case", () => {
    expect(mapSection("   ##\tdestination ##\nOffline.\n## Notes", "Destination")).toBe("Offline.");
  });

  it("skips headings in fenced code, closing a fence only with the same marks", () => {
    const body = "## Destination\n\n~~~~\n## Notes\n~~~\n## Notes\n~~~~\nOffline.\n\n## Notes\n\nNone.";
    expect(mapSection(body, "Destination")).toBe("~~~~\n## Notes\n~~~\n## Notes\n~~~~\nOffline.");
  });

  it("is empty for a missing or empty section, and ignores a level-3 heading of the same name", () => {
    expect(mapSection("## Notes\n\n### Destination\n\nx", "Destination")).toBe("");
    expect(mapSection("## Destination\n\n## Notes", "Destination")).toBe("");
  });
});
