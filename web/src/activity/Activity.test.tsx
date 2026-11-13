// Activity view tests: the real App against FakeApi, at ?view=activity. They
// check what the human sees and does: opening the Activity Dashboard from
// the Inbox's Admin section on the sidebar's Project or All Projects, its
// Projects picker and Back, and that it shows whatever needs the human.
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { App } from "../App";
import { FakeApi } from "../api/fake";
import { loadedDashboard } from "../focus/focusTesting";

function world() {
  const fake = new FakeApi();
  fake.addProject("acme", "Acme");
  fake.addProject("beta", "Beta");
  fake.addTicket("acme", { title: "Login form", state: "Ready" }); // acme-1
  fake.addTicket("acme", { title: "Sign-up page", state: "In Progress" }); // acme-2
  fake.addTicket("beta", { title: "Beta export", state: "In Progress" }); // beta-1
  return fake;
}

function renderApp(fake: FakeApi, url = "/") {
  window.history.replaceState(null, "", url);
  return render(<App api={fake} now={() => fake.now} />);
}

const nav = () => screen.getByRole("navigation", { name: "Filters" });

describe("opening the Activity view from the Inbox", () => {
  it("links from the Admin section, between Usage and CI, on the sidebar's Project", () => {
    renderApp(world(), "/?view=inbox&project=acme&state=Accepted");
    const link = within(nav()).getByRole("link", { name: "Activity" });
    const usage = within(nav()).getByRole("link", { name: "Usage" });
    const ci = within(nav()).getByRole("link", { name: "CI" });
    expect(usage.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link.compareDocumentPosition(ci) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(link).toHaveAttribute("href", "?view=activity&project=acme&back=view%3Dinbox%26project%3Dacme%26state%3DAccepted");
    expect(link).not.toHaveTextContent(/\d/);
  });

  it("links on All Projects when the sidebar has none chosen", () => {
    renderApp(world(), "/?view=inbox");
    expect(within(nav()).getByRole("link", { name: "Activity" })).toHaveAttribute("href", "?view=activity");
  });
});

const activity = () => screen.getByRole("main", { name: "Activity" });
const dashboard = () => loadedDashboard(activity());

describe("the Activity view", () => {
  it("shows the Activity Dashboard of its Project, whatever needs the human and whatever Focus Mode shows when nothing does", async () => {
    const fake = world();
    fake.updateTicket("acme-1", { state: "Failed" });
    fake.setNeedsMe([{ reason: "failed", tickets: [{ id: "acme-1" }] }]);
    await act(() => fake.setOptions({ focus_clear: "all_clear" }));
    renderApp(fake, "/?view=activity&project=acme");
    const d = await dashboard();
    expect(within(within(d).getByRole("list", { name: "In Progress" })).getAllByRole("listitem")).toHaveLength(1);
    expect(d).toHaveTextContent("Sign-up page");
    expect(d).not.toHaveTextContent("Beta export");
  });

  it("opens from the link on the sidebar's Project and Back returns to the Inbox as it was", async () => {
    renderApp(world(), "/?view=inbox&project=acme&state=Accepted");
    await userEvent.click(within(nav()).getByRole("link", { name: "Activity" }));
    expect(within(activity()).getByRole("combobox", { name: "Projects" })).toHaveValue("acme");

    await userEvent.click(within(activity()).getByRole("button", { name: "Back to the Inbox" }));
    expect(window.location.search).toBe("?view=inbox&project=acme&state=Accepted");
  });

  it("changes scope with the Projects picker, keeping the address", async () => {
    renderApp(world(), "/?view=activity");
    const picker = within(activity()).getByRole("combobox", { name: "Projects" });
    await within(picker).findByRole("option", { name: "Beta" });
    await userEvent.selectOptions(picker, "beta");
    expect(window.location.search).toBe("?view=activity&project=beta");
    const d = await dashboard();
    expect(d).toHaveTextContent("Beta export");
    expect(d).not.toHaveTextContent("Sign-up page");
  });

  it("opens an In Progress Ticket in the Inbox", async () => {
    renderApp(world(), "/?view=activity&project=acme");
    await userEvent.click(await within(await dashboard()).findByRole("link", { name: /Sign-up page/ }));
    expect(window.location.search).toContain("view=inbox");
    expect(window.location.search).toContain("ticket=");
  });

  it("returns from the CI view its CI Running block opens to the Activity view, on the same Project", async () => {
    const fake = world();
    await fake.setCITracking("acme", { tracking: true, provider: "github_actions", repository: "acme/web", ignored_workflows: [] });
    renderApp(fake, "/?view=activity&project=acme");
    const d = await dashboard();
    // The block remounts when the count arrives, so click its link only after that.
    await waitFor(() => expect(within(within(d).getByRole("group", { name: "CI Running" })).getByTestId("count")).toHaveTextContent("0"));
    await userEvent.click(within(d).getByRole("link", { name: /CI Running/ }));
    expect(window.location.search).toContain("view=ci");

    await userEvent.click(screen.getByRole("button", { name: "Back to Activity" }));
    expect(window.location.search).toBe("?view=activity&project=acme");
    expect(await dashboard()).toBeInTheDocument();
  });
});

