import { readdirSync, readFileSync } from "node:fs";
import { join, relative, sep } from "node:path";
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

// The landing scan (ADR 0039, ADR 0063): the UI reads the Landing
// capabilities the Project's landing view says, and never branches on a
// Landing Method or Repository Landing itself, as
// TestNoLandingSettingIsReadOutsideThePolicy guards Go.
const landingValue = `["'](?:merge|branch|pull_request|default|merge_push|branch_push)["']`;
const landingPatterns = [
  new RegExp(`[!=]==\\s*${landingValue}`),
  new RegExp(`${landingValue}\\s*[!=]==`),
  /\.method\s*[!=]==/,
  /(?:endsWith|includes)\(\s*["']_push["']\s*\)/,
];
const landingMarker = /\/\/\s*landing:\s*\S/;

// unmarkedLandingTests returns the 1-based line numbers that test a
// landing value without a `// landing: <reason>` comment on the same line
// or the line before.
function unmarkedLandingTests(text: string): number[] {
  const lines = text.split("\n");
  const marked = (line: string | undefined) => line !== undefined && landingMarker.test(line);
  const found: number[] = [];
  lines.forEach((line, i) => {
    if (landingPatterns.some((p) => p.test(line)) && !marked(line) && !marked(lines[i - 1])) found.push(i + 1);
  });
  return found;
}

// landingScanSkips are the files that may test landing values: the
// generated wire types, the contract cases, the scenario builder and the
// Driver, which build and check worlds rather than show them.
const landingScanSkips = ["api/wire.gen.ts", "api/contract/", "api/scenario.ts", "api/driver.ts"];

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    if (!/\.tsx?$/.test(entry.name) || /\.(test|stories)\./.test(entry.name)) return [];
    const rel = relative(__dirname, path).split(sep).join("/");
    return landingScanSkips.some((skip) => rel.startsWith(skip)) ? [] : [path];
  });
}

describe("the landing scan", () => {
  it("fails a test of a landing value", () => {
    expect(unmarkedLandingTests('a\nconst pr = draft.method === "pull_request";\nb')).toEqual([2]);
    expect(unmarkedLandingTests('if (r.landing !== "merge_push") {}')).toEqual([1]);
    expect(unmarkedLandingTests("if ('branch' === l) {}")).toEqual([1]);
    expect(unmarkedLandingTests("const same = a.method === b.method;")).toEqual([1]);
    expect(unmarkedLandingTests('r.landing.endsWith("_push")')).toEqual([1]);
    expect(unmarkedLandingTests('r.landing.includes("_push")')).toEqual([1]);
  });

  it("passes a test marked on the same line or the line before", () => {
    expect(unmarkedLandingTests('checked={draft.method === m.method} // landing: shows the chosen method')).toEqual([]);
    expect(unmarkedLandingTests('// landing: shows the chosen method\nchecked={draft.method === "merge"}')).toEqual([]);
  });

  it("fails a marker with an empty reason", () => {
    expect(unmarkedLandingTests('const pr = draft.method === "pull_request"; // landing:')).toEqual([1]);
    expect(unmarkedLandingTests('// landing:   \nconst pr = draft.method === "pull_request";')).toEqual([2]);
  });

  it("lets other comparisons through", () => {
    expect(unmarkedLandingTests('if (state === "merged") {}\nconst m = methods[draft.method];')).toEqual([]);
  });

  it("finds no unmarked test of a landing value in the real source", () => {
    const failures = sourceFiles(__dirname).flatMap((file) =>
      unmarkedLandingTests(readFileSync(file, "utf8")).map((line) => `${file}:${line}`),
    );
    expect(failures).toEqual([]);
  });
});
