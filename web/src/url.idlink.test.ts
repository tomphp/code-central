// The Inbox's open=<id> and where an ID Link goes from each view.
import { describe, expect, it } from "vitest";
import { hrefFor, idLinkPatch, readLocation, type UiLocation } from "./url";

// go is the address an ID Link to id opens from the address from.
const go = (from: string, id: string) => {
  const loc = readLocation(from);
  return hrefFor({ ...loc, ...idLinkPatch(loc, id) });
};

describe("open= in the Inbox's address", () => {
  it("is read and written for the Inbox", () => {
    expect(readLocation("?view=inbox&open=acme-2")).toEqual(expect.objectContaining({ view: "inbox", open: "acme-2" }));
    expect(hrefFor({ view: "inbox", open: "acme-2" })).toBe("?view=inbox&open=acme-2");
  });

  it("is dropped from every other view", () => {
    const loc: UiLocation = { view: "board", open: "acme-2" };
    expect(hrefFor(loc)).toBe("?view=board");
    expect(readLocation("?view=board&open=acme-2").open).toBeUndefined();
    expect(hrefFor({ view: "focus", open: "acme-2" })).toBe("?view=focus");
  });
});

describe("where an ID Link goes", () => {
  it("on the Board is its popup, keeping the Board's other parameters", () => {
    expect(go("?view=board&project=acme&group=spec&popup=acme-1", "acme-2")).toBe("?view=board&project=acme&group=spec&popup=acme-2");
  });

  it("in the Inbox is open=, clearing the selection", () => {
    expect(go("?view=inbox&state=Ready&requirement=acme-1&ticket=acme-3", "acme-2")).toBe("?view=inbox&state=Ready&open=acme-2");
  });

  it("in the Inbox switches the Project filter to the ID's Project, and leaves All Projects", () => {
    expect(go("?view=inbox&project=acme", "beta-5")).toBe("?view=inbox&project=beta&open=beta-5");
    expect(go("?view=inbox", "beta-5")).toBe("?view=inbox&open=beta-5");
  });

  it("from any other view is the Inbox, in the Project when the view is scoped to one", () => {
    expect(go("?view=focus&project=acme", "beta-5")).toBe("?view=inbox&project=beta&open=beta-5");
    expect(go("?view=usage", "acme-2")).toBe("?view=inbox&open=acme-2");
    expect(go("?view=ci&project=acme", "acme-2")).toBe("?view=inbox&project=acme&open=acme-2");
  });
});
