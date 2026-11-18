import { describe, expect, it } from "vitest";
import { xTickIndexes } from "./LineChart";

describe("xTickIndexes", () => {
  it("labels every point up to eight", () => {
    expect(xTickIndexes(3)).toEqual([0, 1, 2]);
    expect(xTickIndexes(6)).toEqual([0, 1, 2, 3, 4, 5]);
    expect(xTickIndexes(8)).toHaveLength(8);
  });
  it("spaces eight evenly, first and last included, beyond that", () => {
    const t = xTickIndexes(50);
    expect(t).toHaveLength(8);
    expect(t[0]).toBe(0);
    expect(t[7]).toBe(49);
  });
});
