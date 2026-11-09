// New Prototype tests: the real App against FakeApi. "New Prototype" on
// each Spec's view (Requirement, Map and plain Spec), and the plain Spec
// view it sits on.
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const detail = () => screen.getByRole("complementary", { name: "Ticket" });
const params = () => new URLSearchParams(window.location.search);

// prototype asks for a Prototype from the view named view and returns
// the dialog's submit, once the text is typed.
async function prototype(view: string, text: string) {
  const pane = await within(detail()).findByRole("region", { name: view });
  await userEvent.click(within(pane).getByRole("button", { name: "New Prototype" }));
  const modal = screen.getByRole("dialog", { name: "New Prototype" });
  expect(within(modal).getAllByRole("textbox")).toHaveLength(1);
  expect(within(modal).queryByRole("combobox")).toBeNull();
  const submit = within(modal).getByRole("button", { name: "Submit prototype" });
  expect(submit).toBeDisabled();
  await userEvent.type(within(modal).getByRole("textbox", { name: "Prototype" }), text);
  return submit;
}

// wantPrototype checks the new Prototype is a Ready prototype Ticket on
// spec, and is the selected Ticket.
async function wantPrototype(fake: FakeApi, spec: string, id: string, title: string) {
  expect(fake.calls).toContain("submitPrototype");
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  expect(await fake.getTicket("acme", id)).toMatchObject({ title, state: "Ready", spec: { id: spec }, labels: ["prototype"] });
  expect(params().get("requirement")).toBe(spec);
  expect(params().get("ticket")).toBe(id);
  expect(await within(detail()).findByRole("heading", { name: title, level: 2 })).toBeInTheDocument();
}

describe("New Prototype", () => {
  it("is on the Requirement view, and submits a Prototype on that Requirement", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addProject("beta", "Beta");
    await fake.submitRequirement("acme", "Dark mode"); // acme-1, Stages acme-2 to acme-4
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    await userEvent.click(await prototype("Requirement", "A theme picker{Enter}Would a picker beat a toggle?"));
    await wantPrototype(fake, "acme-1", "acme-5", "Prototype: A theme picker");
    expect((await fake.getTicket("acme", "acme-5")).description).toBe("A theme picker\nWould a picker beat a toggle?");
  });

  it("is in the Map view's header, and submits a Prototype on that Map", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    await fake.submitMap("acme", "Offline mode"); // acme-1, Charting acme-2
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    const pane = await within(detail()).findByRole("region", { name: "Map" });
    const button = within(pane).getByRole("button", { name: "New Prototype" });
    expect(button.compareDocumentPosition(within(pane).getByRole("list", { name: "Map Tickets" })) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await userEvent.click(await prototype("Map", "Try a local cache"));
    await wantPrototype(fake, "acme-1", "acme-3", "Prototype: Try a local cache");
    const node = await within(within(pane).getByRole("list", { name: "Map Tickets" })).findByText(/Prototype: Try a local cache/);
    expect(node.closest("[role=listitem]")).toHaveTextContent("prototype");
  });

  it("is on the plain Spec view, and submits a Prototype on that Spec", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addSpec("acme", { title: "Theming", body: "How the app looks." }); // acme-1
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    await userEvent.click(await prototype("Spec", "A theme picker"));
    await wantPrototype(fake, "acme-1", "acme-2", "Prototype: A theme picker");
  });

  it("closes on Cancel without submitting", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addSpec("acme", { title: "Theming" });
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    await prototype("Spec", "A theme picker");
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Cancel" }));
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(fake.calls).not.toContain("submitPrototype");
  });

  it("shows why a Prototype was refused, and keeps the text", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addSpec("acme", { title: "Theming" });
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    const submit = await prototype("Spec", "A theme picker");
    fake.submitPrototype = async () => {
      throw new Error("the Project is archived");
    };
    await userEvent.click(submit);
    const modal = screen.getByRole("dialog", { name: "New Prototype" });
    expect(await within(modal).findByText("the Project is archived")).toBeInTheDocument();
    expect(within(modal).getByRole("textbox", { name: "Prototype" })).toHaveValue("A theme picker");
  });
});

describe("the plain Spec view", () => {
  it("shows a Spec that is neither a Requirement nor a Map: its header and its Tickets, each opening in the Ticket panel", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addSpec("acme", { title: "Theming", body: "How the app looks." }); // acme-1
    const spec = { id: "acme-1", title: "Theming" };
    fake.addTicket("acme", { title: "Dark palette", state: "In Review", spec }); // acme-2
    fake.addTicket("acme", { title: "Light palette", state: "Ready", spec }); // acme-3
    renderApp(fake, "/?view=inbox&requirement=acme-1");

    const pane = await within(detail()).findByRole("region", { name: "Spec" });
    expect(pane).toHaveTextContent("acme-1");
    expect(pane).toHaveTextContent("Spec · submitted just now");
    expect(within(pane).getByRole("heading", { name: "Theming", level: 2 })).toBeInTheDocument();
    const tickets = within(pane).getByRole("list", { name: "Tickets" });
    expect(within(tickets).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["acme-2 Dark palette In Review", "acme-3 Light palette Ready"]);
    await userEvent.click(within(tickets).getByRole("button", { name: /Light palette/ }));
    expect(params().get("ticket")).toBe("acme-3");
    expect(await within(detail()).findByRole("heading", { name: "Light palette", level: 2 })).toBeInTheDocument();
  });

  it("opens from the Spec named in the Ticket panel", async () => {
    const fake = new FakeApi();
    fake.addProject("acme", "Acme");
    fake.addSpec("acme", { title: "Theming" }); // acme-1
    fake.addTicket("acme", { title: "Dark palette", state: "Ready", spec: { id: "acme-1", title: "Theming" } }); // acme-2
    renderApp(fake, "/?view=inbox&ticket=acme-2");

    await userEvent.click(await within(detail()).findByRole("button", { name: "acme-1 Theming" }));
    expect(params().get("requirement")).toBe("acme-1");
    expect(params().get("ticket")).toBe("acme-2");
    expect(await within(detail()).findByRole("region", { name: "Spec" })).toBeInTheDocument();
  });
});
