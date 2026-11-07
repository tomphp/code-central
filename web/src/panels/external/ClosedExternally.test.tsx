// The closed-externally panel against FakeApi: a linked Ticket whose
// external item closed, was deleted or left the query, and the human's
// three ways out: Cancel, Mark Done, Unlink.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { FakeApi } from "../../api/fake";
import type { ExternalState } from "../../api/types";
import { panel } from "./panel";

const URL = "https://github.com/acme/web/issues/7";

function world(external: ExternalState = "closed") {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  const t = fake.addTicket("acme", {
    title: "Issue",
    state: "Ready",
    link: { integration: "github", id: "acme/web#7", url: URL, external_state: external, detached: false },
  }); // acme-1
  fake.setNeedsMe([{ reason: "closed_externally", tickets: [{ id: t.id, link: t.link }] }]);
  return fake;
}

async function renderPanel(fake: FakeApi) {
  const t = await fake.getTicket("acme", "acme-1");
  render(<panel.Component api={fake} project="acme" ticket={t} now={fake.now} />);
  return screen.getByRole("region", { name: "Closed externally" });
}

describe("the closed-externally panel", () => {
  it("is the action panel for a Ticket closed externally", async () => {
    window.history.replaceState(null, "", "/?view=inbox&ticket=acme-1");
    render(<App api={world()} now={() => new Date("2026-11-07T09:00:00Z")} />);
    const slot = await screen.findByRole("region", { name: "Action panel" });
    expect(await within(slot).findByRole("region", { name: "Closed externally" })).toBeInTheDocument();
  });

  it("says what happened and links to the external item", async () => {
    const p = await renderPanel(world("deleted"));
    expect(p).toHaveTextContent("github acme/web#7 was deleted");
    expect(within(p).getByRole("link", { name: "github acme/web#7" })).toHaveAttribute("href", URL);
  });

  it("says when the item left the Integration's query", async () => {
    const p = await renderPanel(world("out_of_scope"));
    expect(p).toHaveTextContent("github acme/web#7 left the Integration's query");
  });

  it("cancels", async () => {
    const fake = world();
    const p = await renderPanel(fake);
    await userEvent.click(within(p).getByRole("button", { name: "Cancel" }));
    expect((await fake.getTicket("acme", "acme-1")).state).toBe("Cancelled");
    expect(fake.calls).toContain("cancel");
  });

  it("marks Done as the human's move to Done", async () => {
    const fake = world();
    const p = await renderPanel(fake);
    await userEvent.click(within(p).getByRole("button", { name: "Mark Done" }));
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Done");
    expect(t.moves.at(-1)).toMatchObject({ to: "Done", actor: "human" });
  });

  it("unlinks", async () => {
    const fake = world();
    const p = await renderPanel(fake);
    await userEvent.click(within(p).getByRole("button", { name: "Unlink" }));
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.link?.detached).toBe(true);
    expect(t.needs_me_reason).toBeUndefined();
  });
});
