import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

describe("the real service guard", () => {
  it("throws on a fetch to port 7733", () => {
    expect(() => fetch("http://127.0.0.1:7733/api/projects")).toThrow(/real Code Central service/);
    expect(() => fetch("http://localhost:7733/")).toThrow(/real Code Central service/);
  });

  it("lets another port through to the network", async () => {
    await expect(fetch("http://127.0.0.1:1/")).rejects.toThrow();
  });
});

const waitPattern = /findBy|findAllBy|waitFor\(/;
const markerPattern = /\/\/\s*waits:\s*\S/;

// unmarkedWaits returns the 1-based line numbers that wait without a
// `// waits: <reason>` comment on the same line or the line before.
function unmarkedWaits(text: string): number[] {
  const lines = text.split("\n");
  const marked = (line: string | undefined) => line !== undefined && markerPattern.test(line);
  const found: number[] = [];
  lines.forEach((line, i) => {
    if (waitPattern.test(line) && !marked(line) && !marked(lines[i - 1])) found.push(i + 1);
  });
  return found;
}

function testFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return testFiles(path);
    return entry.name.endsWith(".test.tsx") ? [path] : [];
  });
}

describe("the waits scan", () => {
  it("fails an unmarked wait", () => {
    expect(unmarkedWaits("a\nawait screen.findByText('x');\nb")).toEqual([2]);
    expect(unmarkedWaits("await screen.findAllByRole('x');")).toEqual([1]);
    expect(unmarkedWaits("await waitFor(() => {});")).toEqual([1]);
  });

  it("passes a wait marked on the same line or the line before", () => {
    expect(unmarkedWaits("await screen.findByText('x'); // waits: the stream is async")).toEqual([]);
    expect(unmarkedWaits("// waits: the stream is async\nawait waitFor(() => {});")).toEqual([]);
  });

  it("fails a marker with an empty reason", () => {
    expect(unmarkedWaits("await screen.findByText('x'); // waits:")).toEqual([1]);
    expect(unmarkedWaits("// waits:   \nawait waitFor(() => {});")).toEqual([2]);
  });

  it("finds no unmarked findBy, findAllBy or waitFor in the real tests", () => {
    const failures = testFiles(__dirname).flatMap((file) =>
      unmarkedWaits(readFileSync(file, "utf8")).map((line) => `${file}:${line}`),
    );
    expect(failures).toEqual([]);
  });
});
