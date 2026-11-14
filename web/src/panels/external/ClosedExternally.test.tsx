// The closed-externally panel on the WASM Core: a linked Ticket whose
// external item closed, was deleted or left the query, and the human's
// three ways out: Cancel, Mark Done, Unlink.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../../App";
import { newWorld } from "../../api/core/core";
import { scenario } from "../../api/scenario";
import type { ExternalState } from "../../api/types";
import { panel } from "./panel";

const URL = "https://github.com/acme/web/issues/7";

async function world(external: ExternalState = "closed") {
  const w = await newWorld();
  const s = scenario(w);
  await s.createProject("acme", "Acme");
  const id = await s.linkTicket("acme", "Issue", { integration: "github", id: "acme/web#7", url: URL }, external);
  return { w, id };
}

type Linked = Awaited<ReturnType<typeof world>>;

async function renderPanel({ w, id }: Linked) {
  const t = await w.api.getTicket("acme", id);
  render(<panel.Component api={w.api} project="acme" ticket={t} now={w.now()} />);
  return screen.getByRole("region", { name: "Closed externally" });
}

describe("the closed-externally panel", () => {
  it("is the action panel for a Ticket closed externally", async () => {
    const { w, id } = await world();
    window.history.replaceState(null, "", `/?view=inbox&ticket=${id}`);
    render(<App api={w.api} now={w.now} />);
    const slot = await screen.findByRole("region", { name: "Action panel" });
    expect(await within(slot).findByRole("region", { name: "Closed externally" })).toBeInTheDocument();
  });

  it("says what happened and links to the external item", async () => {
    const p = await renderPanel(await world("deleted"));
    expect(p).toHaveTextContent("github acme/web#7 was deleted");
    expect(within(p).getByRole("link", { name: "github acme/web#7" })).toHaveAttribute("href", URL);
  });

  it("says when the item left the Integration's query", async () => {
    const p = await renderPanel(await world("out_of_scope"));
    expect(p).toHaveTextContent("github acme/web#7 left the Integration's query");
  });

  it("cancels", async () => {
    const x = await world();
    const p = await renderPanel(x);
    await userEvent.click(within(p).getByRole("button", { name: "Cancel" }));
    expect((await x.w.api.getTicket("acme", x.id)).state).toBe("Cancelled");
    expect(x.w.calls).toContain("cancel");
  });

  it("marks Done as the human's move to Done", async () => {
    const x = await world();
    const p = await renderPanel(x);
    await userEvent.click(within(p).getByRole("button", { name: "Mark Done" }));
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t.state).toBe("Done");
    expect(t.moves.at(-1)).toMatchObject({ to: "Done", actor: "human" });
  });

  it("unlinks", async () => {
    const x = await world();
    const p = await renderPanel(x);
    await userEvent.click(within(p).getByRole("button", { name: "Unlink" }));
    const t = await x.w.api.getTicket("acme", x.id);
    expect(t.link?.detached).toBe(true);
    expect(t.needs_me_reason).toBeUndefined();
  });
});
