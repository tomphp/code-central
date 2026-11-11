// Needs Me and Focus Mode under Pull Request (code-central-391): To merge
// with the pull request's link and "conflicting", Pull request closed with
// Send back and Cancel, and no Land anyway, against FakeApi.
import { within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { FakeApi } from "./api/fake";
import { allClearOn, focus, renderApp } from "./focus/focusTesting";
import { rowDetail } from "./format";

const PR_LINK = "https://github.com/acme/web/pull/7";

async function world(state: "open" | "conflicting" | "closed", reason: "to_merge" | "pull_request_closed") {
  const fake = new FakeApi();
  allClearOn(fake);
  fake.addProject("acme", "Acme");
  fake.addTicket("acme", { title: "Login form", state: "Accepted", branch: "acme-1" }); // acme-1
  await fake.reportPushRemote("acme", "git@github.com:acme/web.git");
  await fake.setLandingMethod("acme", { method: "pull_request", push: false, remote: "origin" });
  await fake.reportPushedCommit("acme", "acme-1", "abc123", "acme-1");
  await fake.reportPullRequest("acme", { branch: "acme-1", number: 7, link: PR_LINK, state });
  fake.setNeedsMe([{ reason, tickets: [{ id: "acme-1", branch: "acme-1" }] }]);
  return fake;
}

const item = () => within(focus()).findByRole("article", { name: "Focus Item" });

describe("To merge under Pull Request", () => {
  it("says conflicting and links the pull request in the Needs Me row", async () => {
    const fake = await world("conflicting", "to_merge");
    const row = (await fake.needsMe()).groups[0].tickets[0];
    expect(rowDetail(fake.now, row)).toMatch(/merge pull request #7, conflicting$/);
    expect(row.pull_request).toMatchObject({ link: PR_LINK, state: "conflicting" });
  });

  it("is a Focus Item linking the pull request, offering Send back to fix with a note", async () => {
    const fake = await world("conflicting", "to_merge");
    renderApp(fake);
    const it = await item();
    expect(within(it).getByText("To merge")).toBeInTheDocument();
    const link = await within(it).findByRole("link", { name: "Pull request 7, conflicting" });
    expect(link).toHaveAttribute("href", PR_LINK);

    const send = within(it).getByRole("button", { name: "Send back to fix" });
    expect(send).toBeDisabled();
    await userEvent.type(within(it).getByLabelText(/Note for the fix/), "Merge main in and resolve src/a.ts");
    await userEvent.click(send);
    expect(fake.calls).toContain("send_back");
    expect((await fake.getTicket("acme", "acme-1")).state).toBe("Ready");
  });
});

describe("Pull request closed", () => {
  it("is a Focus Item linking the pull request with Send back and Cancel", async () => {
    const fake = await world("closed", "pull_request_closed");
    renderApp(fake);
    const it = await item();
    expect(within(it).getByText("Pull request closed", { selector: "span" })).toBeInTheDocument();
    expect(await within(it).findByRole("heading", { name: "Pull request closed" })).toBeInTheDocument();
    expect(await within(it).findByRole("link", { name: "Pull request 7, closed" })).toHaveAttribute("href", PR_LINK);
    expect(within(it).queryByRole("button", { name: /Land anyway/ })).toBeNull();

    const send = within(it).getByRole("button", { name: "Send back" });
    expect(send).toBeDisabled();
    await userEvent.type(within(it).getByLabelText(/Note for the next agent/), "Redo the form");
    await userEvent.click(send);
    const t = await fake.getTicket("acme", "acme-1");
    expect(t.state).toBe("Ready");
  });

  it("cancels", async () => {
    const fake = await world("closed", "pull_request_closed");
    renderApp(fake);
    const it = await item();
    await userEvent.click(await within(it).findByRole("button", { name: "Cancel" }));
    expect((await fake.getTicket("acme", "acme-1")).state).toBe("Cancelled");
  });

  it("describes the row with its pull request", async () => {
    const fake = await world("closed", "pull_request_closed");
    const row = (await fake.needsMe()).groups[0].tickets[0];
    expect(rowDetail(fake.now, row)).toBe("Pull request #7 was closed without merging: send the work back or cancel it");
  });
});
