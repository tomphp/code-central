import { describe, expect, it } from "vitest";
import { chatDefaults } from "./modelPickers";

const row = (model: string, effort: string) => ({ model, effort });

describe("chatDefaults", () => {
  it("takes the model from the picker, then the Chat row, then haiku", () => {
    expect(chatDefaults({ chat: row("sonnet", "") }, { model: "opus" }).model).toBe("opus");
    expect(chatDefaults({ chat: row("sonnet", "") }).model).toBe("sonnet");
    expect(chatDefaults({ chat: row("sonnet", "") }, { model: "" }).model).toBe("sonnet");
    expect(chatDefaults({ chat: row("", "") }).model).toBe("haiku");
  });

  it("takes the effort from the picker, then the Chat row, then none", () => {
    expect(chatDefaults({ chat: row("", "low") }, { effort: "max" }).effort).toBe("max");
    expect(chatDefaults({ chat: row("", "low") }).effort).toBe("low");
    expect(chatDefaults({ chat: row("", "") }).effort).toBe("");
  });

  it("never reads the Project default row", () => {
    const models = { default: row("opus", "high"), chat: row("", ""), kinds: [] };
    expect(chatDefaults(models)).toEqual({ model: "haiku", effort: "" });
  });
});
