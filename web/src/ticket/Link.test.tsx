// The Link badge (ticket 42) on rows and in the detail, the read-only note
// and Unlink, against FakeApi.
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import type { LinkView } from "../api/types";
import { rowDetail } from "../format";
import { TicketDetail } from "./TicketDetail";

const URL = "https://github.com/acme/web/issues/7";
const link: LinkView = { integration: "github", id: "acme/web#7", url: URL, external_state: "open", detached: false };

function world(l: LinkView = link) {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", { title: "Issue", state: "Ready", link: l }); // acme-1
  fake.addTicket("acme", { title: "Local", state: "Ready" }); // acme-2
  return fake;
}

async function renderDetail(fake: FakeApi, id = "acme-1") {
  const t = await fake.getTicket("acme", id);
  render(<TicketDetail api={fake} project="acme" ticket={t} now={fake.now} />);
  return screen.getByRole("article", { name: "Ticket details" });
}

describe("the Link badge", () => {
  it("names the Integration and external ID in the detail, linking to the external item", async () => {
    const d = await renderDetail(world());
    const badge = within(d).getByRole("link", { name: "github acme/web#7" });
    expect(badge).toHaveAttribute("href", URL);
    expect(badge.closest(".meta")).not.toBeNull();
  });

  it("notes the title and description are read-only while linked, and offers Unlink", async () => {
    const fake = world();
    const d = await renderDetail(fake);
    expect(d).toHaveTextContent("The title and description are read-only while linked: change them in github.");
    await userEvent.click(within(d).getByRole("button", { name: "Unlink" }));
    expect((await fake.getTicket("acme", "acme-1")).link?.detached).toBe(true);
    expect(fake.calls).toContain("unlink");
  });

  it("shows a detached Link as unlinked, with no note and no Unlink", async () => {
    const d = await renderDetail(world({ ...link, detached: true }));
    expect(within(d).getByRole("link", { name: "github acme/web#7 (unlinked)" })).toBeInTheDocument();
    expect(d).not.toHaveTextContent("read-only");
    expect(within(d).queryByRole("button", { name: "Unlink" })).toBeNull();
  });

  it("is absent on an unlinked Ticket", async () => {
    const d = await renderDetail(world(), "acme-2");
    expect(within(d).queryByText(/github/)).toBeNull();
    expect(within(d).queryByRole("button", { name: "Unlink" })).toBeNull();
  });

  it("shows on the State list's rows and on Needs Me rows", async () => {
    const fake = world({ ...link, external_state: "closed" });
    fake.setNeedsMe([{ reason: "closed_externally", tickets: [{ id: "acme-1", link: { ...link, external_state: "closed" } }] }]);
    window.history.replaceState(null, "", "/?view=inbox");
    const { unmount } = render(<App api={fake} now={() => fake.now} />);
    const list = await screen.findByRole("main", { name: "Tickets" });
    const row = await within(list).findByText("Issue");
    expect(row.closest("a")).toHaveTextContent("github acme/web#7");
    unmount();

    window.history.replaceState(null, "", "/?view=inbox&project=acme&state=Ready");
    render(<App api={fake} now={() => fake.now} />);
    const rows = await screen.findByRole("main", { name: "Tickets" });
    const linked = (await within(rows).findByText("Issue")).closest("a")!;
    expect(linked).toHaveTextContent("github acme/web#7");
    expect((await within(rows).findByText("Local")).closest("a")).not.toHaveTextContent("github");
  });
});

describe("the closed-externally row detail", () => {
  const now = new Date("2026-11-07T09:00:00Z");
  const row = { project: "acme", id: "acme-1", title: "Issue", state: "Ready" as const, reason: "closed_externally" as const, since: now.toISOString() };
  it("says what happened to which external item", () => {
    expect(rowDetail(now, { ...row, link: { ...link, external_state: "closed" } })).toBe("Closed in github acme/web#7 · still Ready");
    expect(rowDetail(now, { ...row, link: { ...link, external_state: "deleted" } })).toBe("Deleted in github acme/web#7 · still Ready");
    expect(rowDetail(now, { ...row, link: { ...link, external_state: "out_of_scope" } })).toBe(
      "Left github's query (acme/web#7) · still Ready",
    );
    expect(rowDetail(now, row)).toBe("Closed externally · still Ready");
  });
});
