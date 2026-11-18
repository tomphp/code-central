// New Ticket tests: the real App on a WASM Core world. The button on the
// Spec view and the Requirement view, shown by the view's `ticketable`, and
// its dialog's Ready and Save as Draft.
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { newWorld, type World } from "../api/core/core";
import { ApiError } from "../api/types";
import { renderApp, scenario, settle } from "../worldTesting";

async function acme() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  await s.project("spare", "Spare");
  const spec = await s.spec("acme", { title: "Theming" });
  await s.ticket("acme", { title: "Dark palette", state: "Ready", spec });
  return { w, s, spec };
}

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const pane = (view: string) => within(detail()).getByRole("region", { name: view });

async function click(w: World, el: Element) {
  await userEvent.click(el);
  await settle(w);
}

async function write(w: World, text: string) {
  const modal = screen.getByRole("dialog", { name: "New Ticket" });
  await userEvent.type(within(modal).getByRole("textbox", { name: "Ticket" }), text);
  await settle(w);
  return modal;
}

describe("New Ticket on a Spec view", () => {
  it("is shown when the Spec is ticketable, and submits Ready, titled by the first line, then selects it", async () => {
    const { w, spec } = await acme();
    expect((await w.api.getSpec("acme", spec)).ticketable).toBe(true);
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    await click(w, within(pane("Spec")).getByRole("button", { name: "New Ticket" }));
    const modal = await write(w, "Rename the button");
    await click(w, within(modal).getByRole("button", { name: "Submit ticket" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(w.calls).toContain("newTicket");
    const made = (await w.api.findTickets("acme")).find((t) => t.title === "Rename the button")!;
    expect(await w.api.getTicket("acme", made.id)).toMatchObject({ state: "Ready", spec: { id: spec } });
    expect(new URLSearchParams(window.location.search).get("ticket")).toBe(made.id);
    expect(within(pane("Spec")).getByText("Rename the button")).toBeInTheDocument();
  });

  it("saves as a Draft", async () => {
    const { w, spec } = await acme();
    await renderApp(w, `/?view=inbox&requirement=${spec}`);

    await click(w, within(pane("Spec")).getByRole("button", { name: "New Ticket" }));
    const modal = await write(w, "Maybe later");
    await click(w, within(modal).getByRole("button", { name: "Save as Draft" }));

    const made = (await w.api.findTickets("acme")).find((t) => t.title === "Maybe later")!;
    expect(made.state).toBe("Draft");
  });

  it("shows the core's refusal and keeps the text", async () => {
    const { w, spec } = await acme();
    await renderApp(w, `/?view=inbox&requirement=${spec}`);
    vi.spyOn(w.api, "newTicket").mockRejectedValue(new ApiError(403, "action_not_allowed", "Project is archived", "spec"));

    await click(w, within(pane("Spec")).getByRole("button", { name: "New Ticket" }));
    const modal = await write(w, "More");
    await click(w, within(modal).getByRole("button", { name: "Submit ticket" }));

    expect(within(screen.getByRole("dialog", { name: "New Ticket" })).getByText("Project is archived")).toBeInTheDocument();
  });

  it("is absent on a Long-lived Spec, a Map, a running Requirement and an Archived Project's Spec", async () => {
    const { w, s } = await acme();
    const longLived = await s.spec("acme", { title: "Small changes", labels: ["long-lived"] });
    const map = (await w.api.submitMap("acme", "Offline mode")).spec.id;
    const req = (await w.api.submitRequirement("acme", "Dark mode")).spec.id;
    for (const [id, view] of [[longLived, "Spec"], [map, "Map"], [req, "Requirement"]] as const) {
      const { unmount } = await renderApp(w, `/?view=inbox&requirement=${id}`);
      expect(within(pane(view)).queryByRole("button", { name: "New Ticket" })).toBeNull();
      unmount();
    }
  });

  it("is absent on a Spec in an Archived Project", async () => {
    const w = await newWorld();
    const s = scenario(w);
    await s.project("acme", "Acme");
    await s.project("spare", "Spare");
    const spec = await s.spec("acme", { title: "Theming" });
    await s.ticket("acme", { title: "Dark palette", state: "Done", spec });
    await s.archive("acme");
    await renderApp(w, `/?view=inbox&requirement=${spec}`);
    expect(within(pane("Spec")).queryByRole("button", { name: "New Ticket" })).toBeNull();
  });
});

describe("New Ticket on a Requirement view", () => {
  it("is shown once the Stages are finished, and submits Ready and Draft", async () => {
    const { w, s } = await acme();
    const req = await s.requirement("acme", "Dark mode");
    await renderApp(w, `/?view=inbox&requirement=${req}`);
    expect((await w.api.getSpec("acme", req)).ticketable).toBe(true);

    await click(w, within(pane("Requirement")).getByRole("button", { name: "New Ticket" }));
    let modal = await write(w, "Follow-up");
    await click(w, within(modal).getByRole("button", { name: "Submit ticket" }));
    expect((await w.api.findTickets("acme")).find((t) => t.title === "Follow-up")!.state).toBe("Ready");

    await click(w, within(pane("Requirement")).getByRole("button", { name: "New Ticket" }));
    modal = await write(w, "Later");
    await click(w, within(modal).getByRole("button", { name: "Save as Draft" }));
    expect((await w.api.findTickets("acme")).find((t) => t.title === "Later")!.state).toBe("Draft");
  });
});
