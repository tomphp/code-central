// Sizes and counts as the Usage view shows them, a CI Run's duration and
// a short commit. Real calls are too small to reach the big units, and
// real runs too short to reach hours, so those are checked here.
import { describe, expect, it } from "vitest";
import { bytes, count, runDuration, shortCommit } from "./format";

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

describe("shortCommit", () => {
  it("is a commit's first 7 characters", () => {
    expect(shortCommit("abc1234567890")).toBe("abc1234");
  });

  it("leaves a commit of 7 characters or fewer whole", () => {
    expect(shortCommit("abc1234")).toBe("abc1234");
    expect(shortCommit("abc")).toBe("abc");
    expect(shortCommit("")).toBe("");
  });
});

describe("runDuration", () => {
  const now = new Date("2026-03-01T12:00:00Z");
  const running = (started: string) => ({ status: "in_progress" as const, started });

  it("is a dash before the run starts", () => {
    expect(runDuration({ status: "queued" }, now)).toBe("–");
  });

  it("counts seconds, then minutes and seconds, then hours and minutes", () => {
    expect(runDuration(running("2026-03-01T11:59:15Z"), now)).toBe("45s");
    expect(runDuration(running("2026-03-01T11:56:48Z"), now)).toBe("3m 12s");
    expect(runDuration(running("2026-03-01T10:55:00Z"), now)).toBe("1h 5m");
  });

  it("ends a finished run at its finish, and a running one now", () => {
    expect(runDuration({ status: "completed", started: "2026-03-01T11:50:00Z", finished: "2026-03-01T11:53:12Z" }, now)).toBe("3m 12s");
    expect(runDuration(running("2026-03-01T11:50:00Z"), now)).toBe("10m 0s");
  });

  it("never goes below nothing", () => {
    expect(runDuration(running("2026-03-01T12:00:30Z"), now)).toBe("0s");
  });
});
