// The Link badge (ticket 42) on rows and in the detail, the read-only note
// and Unlink, on a WASM Core world.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld } from "../api/core/core";
import { scenario } from "../api/scenario";
import type { ExternalState, LinkView } from "../api/types";
import { rowDetail } from "../format";
import { TicketDetail } from "./TicketDetail";

const URL = "https://github.com/acme/web/issues/7";
const link: LinkView = { integration: "github", id: "acme/web#7", url: URL, external_state: "open", detached: false, outbound: false };

// world has a Ready Ticket linked to an external item, and a local one.
async function world(opts: { external?: ExternalState; detached?: boolean } = {}) {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  const linked = await s.linkedTicket("acme", { title: "Issue", state: "Ready", link: { integration: link.integration, id: link.id, url: link.url, external_state: opts.external } });
  if (opts.detached) await w.api.unlink("acme", linked);
  const local = await s.ticket("acme", { title: "Local" });
  return { w, linked, local };
}

async function renderDetail(w: Awaited<ReturnType<typeof world>>["w"], id: string) {
  const t = await w.api.getTicket("acme", id);
  render(<TicketDetail api={w.api} project="acme" ticket={t} now={w.now()} />);
  return screen.getByRole("article", { name: "Ticket details" });
}

describe("the Link badge", () => {
  it("names the Integration and external ID in the detail, linking to the external item", async () => {
    const { w, linked } = await world();
    const d = await renderDetail(w, linked);
    const badge = within(d).getByRole("link", { name: "github acme/web#7" });
    expect(badge).toHaveAttribute("href", URL);
    expect(badge.closest(".meta")).not.toBeNull();
  });

  it("notes the title and description are read-only while linked, and offers Unlink", async () => {
    const { w, linked } = await world();
    const d = await renderDetail(w, linked);
    expect(d).toHaveTextContent("The title and description are read-only while linked: change them in github.");
    await userEvent.click(within(d).getByRole("button", { name: "Unlink" }));
    expect((await w.api.getTicket("acme", linked)).link?.detached).toBe(true);
  });

  it("shows a detached Link as unlinked, with no note and no Unlink", async () => {
    const { w, linked } = await world({ detached: true });
    const d = await renderDetail(w, linked);
    expect(within(d).getByRole("link", { name: "github acme/web#7 (unlinked)" })).toBeInTheDocument();
    expect(d).not.toHaveTextContent("read-only");
    expect(within(d).queryByRole("button", { name: "Unlink" })).toBeNull();
  });

  it("is absent on an unlinked Ticket", async () => {
    const { w, local } = await world();
    const d = await renderDetail(w, local);
    expect(within(d).queryByText(/github/)).toBeNull();
    expect(within(d).queryByRole("button", { name: "Unlink" })).toBeNull();
  });

  it("shows on the State list's rows and on Needs Me rows", async () => {
    const { w } = await world({ external: "closed" });
    window.history.replaceState(null, "", "/?view=inbox");
    const { unmount } = render(<App api={w.api} now={w.now} />);
    const list = await screen.findByRole("main", { name: "Tickets" });
    const row = await within(list).findByText("Issue");
    expect(row.closest("a")).toHaveTextContent("github acme/web#7");
    unmount();

    window.history.replaceState(null, "", "/?view=inbox&project=acme&state=Ready");
    render(<App api={w.api} now={w.now} />);
    const rows = await screen.findByRole("main", { name: "Tickets" });
    const linked = (await within(rows).findByText("Issue")).closest("a")!;
    expect(linked).toHaveTextContent("github acme/web#7");
    expect((await within(rows).findByText("Local")).closest("a")).not.toHaveTextContent("github");
  });
});

describe("the closed-externally row detail", () => {
  const now = new Date("2026-11-07T09:00:00Z");
  const row = { project: "acme", id: "acme-1", title: "Issue", state: "Ready" as const, reason: "closed_externally" as const, since: now.toISOString(), focus_item: true };
  it("says what happened to which external item", () => {
    expect(rowDetail(now, { ...row, link: { ...link, external_state: "closed" } })).toBe("Closed in github acme/web#7 · still Ready");
    expect(rowDetail(now, { ...row, link: { ...link, external_state: "deleted" } })).toBe("Deleted in github acme/web#7 · still Ready");
    expect(rowDetail(now, { ...row, link: { ...link, external_state: "out_of_scope" } })).toBe(
      "Left github's query (acme/web#7) · still Ready",
    );
    expect(rowDetail(now, row)).toBe("Closed externally · still Ready");
  });
});
