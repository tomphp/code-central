// Edit tests: the real App against FakeApi. Edit on a Draft Ticket in the
// Ticket panel, and the body section, with Edit, on the Requirement view
// and the plain Spec view.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import type { LinkView } from "../api/types";

function renderApp(fake: FakeApi, url: string) {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const ticketUrl = (id = "acme-2") => `/?view=inbox&requirement=acme-1&ticket=${id}`;

function draft(extra: Partial<Parameters<FakeApi["addTicket"]>[1]> = {}, project = "acme") {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme", { archived: project === "archived" });
  fake.addSpec("acme", { title: "Theming" }); // acme-1
  fake.addTicket("acme", { title: "Dark palette", state: "Draft", description: "Use **navy**.", spec: { id: "acme-1", title: "Theming" }, ...extra }); // acme-2
  return fake;
}

async function openTicket(fake: FakeApi, title = "Dark palette") {
  renderApp(fake, ticketUrl());
  await within(detail()).findByRole("heading", { name: title, level: 2 });
}

// The Ticket's own Edit, in its header: the Spec view above has one too.
const edit = () => within(detail().querySelector<HTMLElement>(".ticket-head")!).queryByRole("button", { name: "Edit" });

describe("Edit on a Ticket", () => {
  it("is offered on a Draft Ticket, and changes the title and description together", async () => {
    const fake = draft();
    await openTicket(fake);
    await userEvent.click(edit()!);
    const title = within(detail()).getByRole("textbox", { name: "Title" });
    expect(title).toHaveValue("Dark palette");
    expect(within(detail()).getByRole("textbox", { name: "Description" })).toHaveValue("Use **navy**.");
    await userEvent.clear(title);
    await userEvent.type(title, "Night palette");
    const text = within(detail()).getByRole("textbox", { name: "Description" });
    await userEvent.clear(text);
    await userEvent.type(text, "Use slate.");
    await userEvent.click(within(detail()).getByRole("button", { name: "Save" }));

    expect(await within(detail()).findByRole("heading", { name: "Night palette", level: 2 })).toBeInTheDocument();
    expect(within(detail()).getByText("Use slate.")).toBeInTheDocument();
    expect(within(detail()).queryByRole("textbox", { name: "Title" })).toBeNull();
    expect(within(detail()).getByText("title and description edited by human")).toBeInTheDocument();
    await userEvent.click(within(detail()).getByText(/^History/));
    await userEvent.click(within(detail()).getByRole("button", { name: "show previous" }));
    expect(within(detail()).getByText("Dark palette")).toBeInTheDocument();
  });

  it("sends only the field that changed", async () => {
    const fake = draft();
    await openTicket(fake);
    await userEvent.click(edit()!);
    await userEvent.type(within(detail()).getByRole("textbox", { name: "Description" }), " More.");
    await userEvent.click(within(detail()).getByRole("button", { name: "Save" }));
    expect(await within(detail()).findByText("description edited by human")).toBeInTheDocument();
    expect(within(detail()).queryByText("title and description edited by human")).toBeNull();
  });

  it("writes nothing when Save changes nothing", async () => {
    const fake = draft();
    await openTicket(fake);
    await userEvent.click(edit()!);
    await userEvent.click(within(detail()).getByRole("button", { name: "Save" }));
    expect(within(detail()).queryByRole("textbox", { name: "Title" })).toBeNull();
    expect(fake.calls).not.toContain("edit_ticket");
    expect((await fake.getTicket("acme", "acme-2")).edits).toEqual([]);
  });

  it("drops the draft on Cancel", async () => {
    const fake = draft();
    await openTicket(fake);
    await userEvent.click(edit()!);
    await userEvent.type(within(detail()).getByRole("textbox", { name: "Title" }), " v2");
    await userEvent.click(within(detail()).getByRole("button", { name: "Cancel" }));
    expect(within(detail()).getByRole("heading", { name: "Dark palette", level: 2 })).toBeInTheDocument();
    await userEvent.click(edit()!);
    expect(within(detail()).getByRole("textbox", { name: "Title" })).toHaveValue("Dark palette");
    expect(fake.calls).not.toContain("edit_ticket");
  });

  it("previews the Markdown, and goes back to the textarea with the text kept", async () => {
    const fake = draft();
    await openTicket(fake);
    await userEvent.click(edit()!);
    await userEvent.type(within(detail()).getByRole("textbox", { name: "Description" }), " And _more_.");
    await userEvent.click(within(detail()).getByRole("button", { name: "Preview" }));
    expect(within(detail()).queryByRole("textbox", { name: "Description" })).toBeNull();
    expect(within(detail()).getByText("navy").tagName).toBe("STRONG");
    expect(within(detail()).getByText("more").tagName).toBe("EM");
    await userEvent.click(within(detail()).getByRole("button", { name: "Preview" }));
    expect(within(detail()).getByRole("textbox", { name: "Description" })).toHaveValue("Use **navy**. And _more_.");
  });

  it("refuses an empty title with the service's reason, keeping the form and the text", async () => {
    const fake = draft();
    await openTicket(fake);
    await userEvent.click(edit()!);
    await userEvent.clear(within(detail()).getByRole("textbox", { name: "Title" }));
    await userEvent.type(within(detail()).getByRole("textbox", { name: "Description" }), " Extra.");
    await userEvent.click(within(detail()).getByRole("button", { name: "Save" }));
    expect(await within(detail()).findByRole("alert")).toHaveTextContent("a Ticket needs a title");
    expect(within(detail()).getByRole("textbox", { name: "Description" })).toHaveValue("Use **navy**. Extra.");
    expect((await fake.getTicket("acme", "acme-2")).edits).toEqual([]);
  });

  it("keeps the form and the draft when the Ticket leaves Draft meanwhile, and Save still goes through", async () => {
    const fake = draft();
    await openTicket(fake);
    await userEvent.click(edit()!);
    await userEvent.type(within(detail()).getByRole("textbox", { name: "Description" }), " Mine.");
    fake.updateTicket("acme-2", { state: "Ready" });
    await waitFor(() => expect(within(detail()).getByText("Ready")).toBeInTheDocument());
    expect(within(detail()).getByRole("textbox", { name: "Description" })).toHaveValue("Use **navy**. Mine.");
    await userEvent.click(within(detail()).getByRole("button", { name: "Save" }));
    expect(await within(detail()).findByText("description edited by human")).toBeInTheDocument();
    expect(within(detail()).getByRole("article", { name: "Ticket details" })).toHaveTextContent("Use navy. Mine.");
    expect(edit()).toBeNull();
  });

  it.each([["Ready"], ["Failed"], ["In Review"], ["Done"]] as const)("is not offered on a %s Ticket", async (state) => {
    const fake = draft({ state });
    await openTicket(fake);
    expect(edit()).toBeNull();
  });

  it("is not offered on a Ticket whose Link owns its definition, but is once it is detached or outbound", async () => {
    const link: LinkView = { integration: "github", id: "acme/web#7", external_state: "open", detached: false, outbound: false };
    await openTicket(draft({ link }));
    expect(edit()).toBeNull();
    expect(within(detail()).getByText(/read-only while linked/)).toBeInTheDocument();
  });

  it.each([[{ detached: true, outbound: false }], [{ detached: false, outbound: true }]])("is offered when the Link is %o", async (flags) => {
    const link: LinkView = { integration: "github", id: "acme/web#7", external_state: "open", ...flags };
    await openTicket(draft({ link }));
    expect(edit()).not.toBeNull();
  });

  it("is not offered in an Archived Project", async () => {
    await openTicket(draft({}, "archived"));
    expect(edit()).toBeNull();
  });
});

describe("the Requirement view's body", () => {
  async function requirement(finishSpecStage = false) {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    const { stages } = await fake.submitRequirement("acme", "Offline mode\nWorks on a plane.");
    if (finishSpecStage) fake.updateTicket(stages[1].id, { state: "Done" });
    renderApp(fake, "/?view=inbox&requirement=acme-1");
    const pane = await within(detail()).findByRole("region", { name: /Requirement's text|Spec/ });
    return { fake, pane };
  }

  it("is collapsed by default, headed Requirement's text, and shows the text when opened", async () => {
    const { pane } = await requirement();
    expect(within(pane).queryByText(/Works on a plane\./)).toBeNull();
    await userEvent.click(within(pane).getByRole("button", { name: "Requirement's text" }));
    expect(within(pane).getByText(/Works on a plane\./)).toBeInTheDocument();
  });

  it("is headed Spec once the spec Stage is Done", async () => {
    const { pane } = await requirement(true);
    expect(within(pane).getByRole("button", { name: "Spec" })).toBeInTheDocument();
  });

  it("is edited through Save, which updates the heading and the body", async () => {
    const { fake, pane } = await requirement();
    await userEvent.click(within(pane).getByRole("button", { name: "Edit" }));
    expect(within(pane).getByRole("textbox", { name: "Body" })).toHaveValue("Offline mode\nWorks on a plane.");
    const title = within(pane).getByRole("textbox", { name: "Title" });
    await userEvent.clear(title);
    await userEvent.type(title, "Flight mode");
    await userEvent.type(within(pane).getByRole("textbox", { name: "Body" }), "\nAnd trains.");
    await userEvent.click(within(pane).getByRole("button", { name: "Save" }));
    expect(await within(detail()).findByRole("heading", { name: "Flight mode", level: 2 })).toBeInTheDocument();
    expect(within(pane).getByText("And trains.", { exact: false })).toBeInTheDocument();
    expect((await fake.getSpec("acme", "acme-1")).edits.map((e) => e.actor)).toEqual(["human"]);
  });

  it("offers Edit until every Ticket is finished, and none after", async () => {
    const { fake, pane } = await requirement();
    expect(within(pane).getByRole("button", { name: "Edit" })).toBeInTheDocument();
    for (const id of ["acme-2", "acme-3", "acme-4"]) fake.updateTicket(id, { state: "Cancelled" });
    await waitFor(() => expect(within(pane).queryByRole("button", { name: "Edit" })).toBeNull());
    await userEvent.click(within(pane).getByRole("button", { name: "Requirement's text" }));
    expect(within(pane).getByText(/Works on a plane\./)).toBeInTheDocument();
  });
});

describe("the plain Spec view's body", () => {
  function world(extra: { link?: LinkView; labels?: string[]; tickets?: "none" | "finished" } = {}, archived = false) {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme", { archived });
    fake.addSpec("acme", { title: "Theming", body: "How the app looks.", ...(extra.link && { link: extra.link }), ...(extra.labels && { labels: extra.labels }) }); // acme-1
    if (extra.tickets === "finished") fake.addTicket("acme", { title: "Palette", state: "Done", spec: { id: "acme-1", title: "Theming" } });
    return fake;
  }
  async function open(fake: FakeApi) {
    renderApp(fake, "/?view=inbox&requirement=acme-1");
    return within(detail()).findByRole("region", { name: "Body" });
  }

  it("is a Body section with Edit on a Spec with no Tickets, closed until opened", async () => {
    const pane = await open(world());
    expect(within(pane).queryByText("How the app looks.")).toBeNull();
    await userEvent.click(within(pane).getByRole("button", { name: "Body" }));
    expect(within(pane).getByText("How the app looks.")).toBeInTheDocument();
    await userEvent.click(within(pane).getByRole("button", { name: "Edit" }));
    await userEvent.click(within(pane).getByRole("button", { name: "Cancel" }));
    expect(within(pane).getByText("How the app looks.")).toBeInTheDocument();
  });

  it("reads No body. for an empty body", async () => {
    const fake = world();
    fake.addSpec("acme", { title: "Empty" }); // acme-2
    renderApp(fake, "/?view=inbox&requirement=acme-2");
    const pane = await within(detail()).findByRole("region", { name: "Body" });
    await userEvent.click(within(pane).getByRole("button", { name: "Body" }));
    expect(within(pane).getByText("No body.")).toBeInTheDocument();
  });

  it("shows a Finished Spec's body with no Edit", async () => {
    const pane = await open(world({ tickets: "finished" }));
    expect(within(pane).queryByRole("button", { name: "Edit" })).toBeNull();
    await userEvent.click(within(pane).getByRole("button", { name: "Body" }));
    expect(within(pane).getByText("How the app looks.")).toBeInTheDocument();
  });

  it("offers no Edit on a definition-owned linked Spec, but does on a detached one", async () => {
    const link: LinkView = { integration: "github", id: "acme/web#7", external_state: "open", detached: false, outbound: false };
    expect(within(await open(world({ link }))).queryByRole("button", { name: "Edit" })).toBeNull();
  });

  it("offers Edit on a detached linked Spec", async () => {
    const link: LinkView = { integration: "github", id: "acme/web#7", external_state: "open", detached: true, outbound: false };
    expect(within(await open(world({ link }))).getByRole("button", { name: "Edit" })).toBeInTheDocument();
  });

  it("offers no Edit in an Archived Project", async () => {
    expect(within(await open(world({}, true))).queryByRole("button", { name: "Edit" })).toBeNull();
  });

  it("has no body section on a Map", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.submitMap("acme", "Plan the year");
    renderApp(fake, "/?view=inbox&requirement=acme-1");
    await within(detail()).findByRole("region", { name: "Map" });
    expect(within(detail()).queryByRole("button", { name: "Edit" })).toBeNull();
    expect(within(detail()).queryByRole("region", { name: "Body" })).toBeNull();
  });
});
