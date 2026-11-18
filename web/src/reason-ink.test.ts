import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { NEEDS_ME_REASONS } from "./api/wire.gen";

describe("Needs Me reason inks", () => {
  it("gives every reason a .reason-<x> rule that sets its line ink", () => {
    const css = readFileSync(join(process.cwd(), "src/styles.css"), "utf8");
    const missing = NEEDS_ME_REASONS.filter(
      (r) => !new RegExp(`\\.reason-${r}\\s*\\{[^}]*--rc:`).test(css),
    );
    expect(missing).toEqual([]);
  });
});
