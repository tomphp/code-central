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
