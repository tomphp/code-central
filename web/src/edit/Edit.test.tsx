// Edit tests: the real App on a WASM Core world. Edit on a Draft Ticket in the
// Ticket panel, and the body section, with Edit, on the Requirement view
// and the plain Spec view.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { newWorld, type World } from "../api/core/core";
import { scenario } from "../api/scenario";
import type { State } from "../api/types";

async function setup() {
  const w = await newWorld();
  const s = scenario(w);
  await s.project("acme", "Acme");
  return { w, s };
}

function renderApp(w: World, url: string) {
  window.history.replaceState(null, "", url);
  return render(<App api={w.api} now={w.now} />);
}

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const specUrl = (spec: string) => `/?view=inbox&requirement=${spec}`;

const GH = { integration: "github", id: "acme/web#7", url: "https://github.com/acme/web/issues/7" };

// draft is the Ticket "Dark palette" under the Spec "Theming", a Draft
// unless state says, or one linked to a GitHub issue, detached from it or not.
async function draft(extra: { state?: State; link?: "owned" | "detached" } = {}) {
  const { w, s } = await setup();
  const spec = await s.spec("acme", { title: "Theming" });
  let id: string;
  if (extra.link) {
    id = (await s.integration(`${s.projectPath("acme")}/integration/create_linked_ticket`, { spec, title: "Dark palette", description: "Use **navy**.", link: GH })).id;
    if (extra.link === "detached") await w.api.unlink("acme", id);
  } else {
    id = await s.ticket("acme", { title: "Dark palette", state: extra.state ?? "Draft", description: "Use **navy**.", spec });
  }
  return { w, s, spec, id };
}

async function openTicket(d: { w: World; spec: string; id: string }, title = "Dark palette") {
  renderApp(d.w, `/?view=inbox&requirement=${d.spec}&ticket=${d.id}`);
  await within(detail()).findByRole("heading", { name: title, level: 2 });
}

// The Ticket's own Edit, in its header: the Spec view above has one too.
const edit = () => within(detail().querySelector<HTMLElement>(".ticket-head")!).queryByRole("button", { name: "Edit" });

describe("Edit on a Ticket", () => {
  it("is offered on a Draft Ticket, and changes the title and description together", async () => {
    const d = await draft();
    await openTicket(d);
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
    const d = await draft();
    await openTicket(d);
    await userEvent.click(edit()!);
    await userEvent.type(within(detail()).getByRole("textbox", { name: "Description" }), " More.");
    await userEvent.click(within(detail()).getByRole("button", { name: "Save" }));
    expect(await within(detail()).findByText("description edited by human")).toBeInTheDocument();
    expect(within(detail()).queryByText("title and description edited by human")).toBeNull();
  });

  it("writes nothing when Save changes nothing", async () => {
    const d = await draft();
    await openTicket(d);
    await userEvent.click(edit()!);
    await userEvent.click(within(detail()).getByRole("button", { name: "Save" }));
    expect(within(detail()).queryByRole("textbox", { name: "Title" })).toBeNull();
    expect(d.w.actions()).not.toContain("edit_ticket");
    expect((await d.w.api.getTicket("acme", d.id)).edits).toEqual([]);
  });

  it("drops the draft on Cancel", async () => {
    const d = await draft();
    await openTicket(d);
    await userEvent.click(edit()!);
    await userEvent.type(within(detail()).getByRole("textbox", { name: "Title" }), " v2");
    await userEvent.click(within(detail()).getByRole("button", { name: "Cancel" }));
    expect(within(detail()).getByRole("heading", { name: "Dark palette", level: 2 })).toBeInTheDocument();
    await userEvent.click(edit()!);
    expect(within(detail()).getByRole("textbox", { name: "Title" })).toHaveValue("Dark palette");
    expect(d.w.actions()).not.toContain("edit_ticket");
  });

  it("previews the Markdown, and goes back to the textarea with the text kept", async () => {
    const d = await draft();
    await openTicket(d);
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
    const d = await draft();
    await openTicket(d);
    await userEvent.click(edit()!);
    await userEvent.clear(within(detail()).getByRole("textbox", { name: "Title" }));
    await userEvent.type(within(detail()).getByRole("textbox", { name: "Description" }), " Extra.");
    await userEvent.click(within(detail()).getByRole("button", { name: "Save" }));
    expect(await within(detail()).findByRole("alert")).toHaveTextContent("a Ticket needs a title");
    expect(within(detail()).getByRole("textbox", { name: "Description" })).toHaveValue("Use **navy**. Extra.");
    expect((await d.w.api.getTicket("acme", d.id)).edits).toEqual([]);
  });

  it("keeps the form and the draft when the Ticket leaves Draft meanwhile, and Save still goes through", async () => {
    const d = await draft();
    await openTicket(d);
    await userEvent.click(edit()!);
    await userEvent.type(within(detail()).getByRole("textbox", { name: "Description" }), " Mine.");
    await d.s.human(`${d.s.ticketPath("acme", d.id)}/move`, { to_state: "Ready" });
    await waitFor(() => expect(within(detail()).getByText("Ready")).toBeInTheDocument());
    expect(within(detail()).getByRole("textbox", { name: "Description" })).toHaveValue("Use **navy**. Mine.");
    await userEvent.click(within(detail()).getByRole("button", { name: "Save" }));
    expect(await within(detail()).findByText("description edited by human")).toBeInTheDocument();
    expect(within(detail()).getByRole("article", { name: "Ticket details" })).toHaveTextContent("Use navy. Mine.");
    expect(edit()).toBeNull();
  });

  it.each([["Ready"], ["Failed"], ["In Review"], ["Done"]] as const)("is not offered on a %s Ticket", async (state) => {
    const d = await draft({ state });
    await openTicket(d);
    expect(edit()).toBeNull();
  });

  it("is not offered on a Ticket whose Link owns its definition, but is once it is detached", async () => {
    await openTicket(await draft({ link: "owned" }));
    expect(edit()).toBeNull();
    expect(within(detail()).getByText(/read-only while linked/)).toBeInTheDocument();
  });

  it("is offered when the Link is detached", async () => {
    await openTicket(await draft({ link: "detached" }));
    expect(edit()).not.toBeNull();
  });
});

describe("the Requirement view's body", () => {
  async function requirement(finishSpecStage = false) {
    const { w, s } = await setup();
    const { spec, stages } = await w.api.submitRequirement("acme", "Offline mode\nWorks on a plane.");
    if (finishSpecStage) {
      // The Stages go in order: grilling, then spec.
      for (const [i, { id }] of stages.slice(0, 2).entries()) {
        await s.finish("acme", id, "Done; implement, it is small", i === 1 ? { recommendation: "implement" } : undefined);
        await s.human(`${s.ticketPath("acme", id)}/accept`, {});
      }
    }
    renderApp(w, specUrl(spec.id));
    const pane = await within(detail()).findByRole("region", { name: /Requirement's text|Spec/ });
    return { w, s, spec, stages, pane };
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
    const { w, spec, pane } = await requirement();
    await userEvent.click(within(pane).getByRole("button", { name: "Edit" }));
    expect(within(pane).getByRole("textbox", { name: "Body" })).toHaveValue("Offline mode\nWorks on a plane.");
    const title = within(pane).getByRole("textbox", { name: "Title" });
    await userEvent.clear(title);
    await userEvent.type(title, "Flight mode");
    await userEvent.type(within(pane).getByRole("textbox", { name: "Body" }), "\nAnd trains.");
    await userEvent.click(within(pane).getByRole("button", { name: "Save" }));
    expect(await within(detail()).findByRole("heading", { name: "Flight mode", level: 2 })).toBeInTheDocument();
    expect(within(pane).getByText("And trains.", { exact: false })).toBeInTheDocument();
    expect((await w.api.getSpec("acme", spec.id)).edits!.map((e) => e.actor)).toEqual(["human"]);
  });

  it("offers Edit until every Ticket is finished, and none after", async () => {
    const { s, stages, pane } = await requirement();
    expect(within(pane).getByRole("button", { name: "Edit" })).toBeInTheDocument();
    for (const { id } of stages) await s.human(`${s.ticketPath("acme", id)}/cancel`, {});
    await waitFor(() => expect(within(pane).queryByRole("button", { name: "Edit" })).toBeNull());
    await userEvent.click(within(pane).getByRole("button", { name: "Requirement's text" }));
    expect(within(pane).getByText(/Works on a plane\./)).toBeInTheDocument();
  });
});

describe("the plain Spec view's body", () => {
  // world is the Spec "Theming" with a body, linked to a GitHub issue
  // (detached from it, or not) when extra says, with a Done Ticket when
  // tickets is "finished", in a Project archived if it can be.
  async function world(extra: { link?: "owned" | "detached"; tickets?: "none" | "finished" } = {}, archived = false) {
    const { w, s } = await setup();
    let spec: string;
    if (extra.link) {
      spec = (await s.integration(`${s.projectPath("acme")}/integration/create_linked_spec`, { title: "Theming", body: "How the app looks.", link: GH })).id;
      if (extra.link === "detached") await w.api.unlink("acme", spec);
    } else {
      spec = await s.spec("acme", { title: "Theming", body: "How the app looks." });
    }
    if (extra.tickets === "finished") await s.ticket("acme", { title: "Palette", state: "Done", spec });
    if (archived) await w.api.archiveProject("acme");
    return { w, s, spec };
  }
  async function open({ w, spec }: { w: World; spec: string }) {
    renderApp(w, specUrl(spec));
    return within(detail()).findByRole("region", { name: "Body" });
  }

  it("is a Body section with Edit on a Spec with no Tickets, closed until opened", async () => {
    const pane = await open(await world());
    expect(within(pane).queryByText("How the app looks.")).toBeNull();
    await userEvent.click(within(pane).getByRole("button", { name: "Body" }));
    expect(within(pane).getByText("How the app looks.")).toBeInTheDocument();
    await userEvent.click(within(pane).getByRole("button", { name: "Edit" }));
    await userEvent.click(within(pane).getByRole("button", { name: "Cancel" }));
    expect(within(pane).getByText("How the app looks.")).toBeInTheDocument();
  });

  it("reads No body. for an empty body", async () => {
    const { w, s } = await world();
    const empty = await s.spec("acme", { title: "Empty" });
    renderApp(w, specUrl(empty));
    const pane = await within(detail()).findByRole("region", { name: "Body" });
    await userEvent.click(within(pane).getByRole("button", { name: "Body" }));
    expect(within(pane).getByText("No body.")).toBeInTheDocument();
  });

  it("shows a Finished Spec's body with no Edit", async () => {
    const pane = await open(await world({ tickets: "finished" }));
    expect(within(pane).queryByRole("button", { name: "Edit" })).toBeNull();
    await userEvent.click(within(pane).getByRole("button", { name: "Body" }));
    expect(within(pane).getByText("How the app looks.")).toBeInTheDocument();
  });

  it("offers no Edit on a definition-owned linked Spec", async () => {
    expect(within(await open(await world({ link: "owned" }))).queryByRole("button", { name: "Edit" })).toBeNull();
  });

  it("offers Edit on a detached linked Spec", async () => {
    expect(within(await open(await world({ link: "detached" }))).getByRole("button", { name: "Edit" })).toBeInTheDocument();
  });

  it("offers no Edit in an Archived Project", async () => {
    expect(within(await open(await world({}, true))).queryByRole("button", { name: "Edit" })).toBeNull();
  });

  it("has no body section on a Map", async () => {
    const { w } = await setup();
    const { spec } = await w.api.submitMap("acme", "Plan the year");
    renderApp(w, specUrl(spec.id));
    await within(detail()).findByRole("region", { name: "Map" });
    expect(within(detail()).queryByRole("button", { name: "Edit" })).toBeNull();
    expect(within(detail()).queryByRole("region", { name: "Body" })).toBeNull();
  });
});
