import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Button } from "./Button";

describe("Button", () => {
  it.each([
    ["default", "btn"],
    ["primary", "btn primary"],
    ["danger", "btn danger"],
  ] as const)("renders the %s variant as a button with class %s", (variant, classes) => {
    render(<Button variant={variant}>Go</Button>);
    expect(screen.getByRole("button", { name: "Go" }).className).toBe(classes);
  });

  it("defaults to the default variant and passes native props and extra classes through", () => {
    render(
      <Button className="extra" disabled type="submit">
        Go
      </Button>,
    );
    const b = screen.getByRole("button", { name: "Go" }) as HTMLButtonElement;
    expect(b.className).toBe("btn extra");
    expect(b.disabled).toBe(true);
    expect(b.type).toBe("submit");
  });
});
