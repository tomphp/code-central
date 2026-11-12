// The Activity view's address: ?view=activity&project=<key>&back=<query>,
// the CI view's shape, for the Activity Dashboard the Inbox's Activity link
// opens.
import { describe, expect, it } from "vitest";
import { hrefFor, readLocation } from "./url";

describe("the Activity view's address", () => {
  it("reads and writes its Project and the Inbox location it returns to", () => {
    const back = { view: "inbox" as const, project: "acme", state: "Accepted" as const };
    const href = "?view=activity&project=acme&back=view%3Dinbox%26project%3Dacme%26state%3DAccepted";
    expect(readLocation(href)).toEqual({ view: "activity", project: "acme", back });
    expect(hrefFor({ view: "activity", project: "acme", back })).toBe(href);
  });

  it("means All Projects and the Inbox when it names neither", () => {
    expect(readLocation("?view=activity")).toEqual({ view: "activity", project: undefined, back: { ...readLocation("?view=inbox") } });
    expect(hrefFor({ view: "activity" })).toBe("?view=activity");
  });

  it("returns to the Inbox when back names any other view", () => {
    expect(readLocation("?view=activity&back=view%3Dusage").back).toEqual(readLocation("?view=inbox"));
  });
});
