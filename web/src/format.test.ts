// Sizes and counts as the Usage view shows them. Real calls are too small
// to reach the big units, so those are checked here.
import { describe, expect, it } from "vitest";
import { bytes, count } from "./format";

describe("bytes", () => {
  it("counts bytes under 1 KB, then KB, then MB, to one decimal place", () => {
    expect(bytes(300)).toBe("300 B");
    expect(bytes(1_023)).toBe("1023 B");
    expect(bytes(1_024)).toBe("1 KB");
    expect(bytes(23_986)).toBe("23.4 KB");
    expect(bytes(6_000_000)).toBe("5.7 MB");
  });
});

describe("count", () => {
  it("separates thousands", () => {
    expect(count(3)).toBe("3");
    expect(count(1_200)).toBe("1,200");
    expect(count(2_000_000)).toBe("2,000,000");
  });
});
