// The Board view's address:
// ?view=board&project=<key>&group=spec&hide=<States>&back=<query>, where
// hide tells "absent" (the browser's pick, else the default) from an empty
// hide= (every column shown).
import { describe, expect, it } from "vitest";
import { hrefFor, readLocation } from "./url";

describe("the Board view's address", () => {
  it("reads and writes its Project, grouping, hidden columns and the Inbox location it returns to", () => {
    const back = readLocation("?view=inbox&project=acme&list=tickets");
    const href = "?view=board&project=acme&group=spec&hide=Draft%2CDone&back=view%3Dinbox%26project%3Dacme%26list%3Dtickets";
    expect(readLocation(href)).toEqual({ view: "board", project: "acme", group: "spec", hide: ["Draft", "Done"], back });
    expect(hrefFor({ view: "board", project: "acme", group: "spec", hide: ["Draft", "Done"], back })).toBe(href);
  });

  it("means All Projects, By Ticket, no hidden columns of its own and the Inbox when it names none", () => {
    expect(readLocation("?view=board")).toEqual({
      view: "board",
      project: undefined,
      group: undefined,
      hide: undefined,
      back: readLocation("?view=inbox"),
    });
    expect(hrefFor({ view: "board" })).toBe("?view=board");
  });

  it("keeps an empty hide=, every column shown, apart from no hide at all", () => {
    expect(readLocation("?view=board&hide=").hide).toEqual([]);
    expect(hrefFor({ view: "board", hide: [] })).toBe("?view=board&hide=");
  });

  it("reads only the States it knows, in workflow order", () => {
    expect(readLocation("?view=board&hide=Cancelled,Bogus,In%20Progress,Ready").hide).toEqual(["Ready", "In Progress", "Cancelled"]);
  });

  it("returns to the Inbox when back names any other view", () => {
    expect(readLocation("?view=board&back=view%3Dusage").back).toEqual(readLocation("?view=inbox"));
  });
});
