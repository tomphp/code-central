// Usage view tests: the Storage tab (Spec code-central-1032).
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { Api } from "../api/api";
import type { StorageView } from "../api/types";
import { bytes } from "../format";
import { settle } from "../worldTesting";
import { projects, world, renderApp, usage, scope, openTab, tab } from "./Usage.testutil";

const storageTable = (name: string) => within(usage()).getByRole("table", { name });
// rows is each body row of the table named, its cells' text joined by " | ".
function rows(name: string): string[] {
  return within(storageTable(name))
    .getAllByRole("row")
    .slice(1)
    .map((r) => [...r.children].map((c) => c.textContent).join(" | "));
}
// figure is the cell beside the row header named in the table named.
function figure(table: string, row: string): string {
  return within(within(storageTable(table)).getByRole("row", { name: new RegExp(`^${row}`) })).getByRole("cell").textContent ?? "";
}

// counting counts api's storage reads.
function counting(api: Api) {
  let reads = 0;
  const counted: Api = {
    ...api,
    storage: (project) => {
      reads++;
      return api.storage(project);
    },
  };
  return { api: counted, reads: () => reads };
}

describe("the Storage tab", () => {
  async function withRecords() {
    const { w, s } = await projects();
    await s.ticket("beta", { title: "Second" });
    await s.ticket("beta", { title: "Third" });
    await s.spec("beta", { title: "Beta's Spec" });
    return w;
  }

  it("hides the range, which comes back on leaving it", async () => {
    const w = await withRecords();
    await renderApp(w, "/?view=usage");
    await openTab("Storage");
    expect(window.location.search).toBe("?view=usage&tab=storage");
    expect(within(usage()).queryByRole("combobox", { name: "Range" })).toBeNull();
    await openTab("Token usage");
    expect(within(usage()).getByRole("combobox", { name: "Range" })).toBeInTheDocument();
  });

  it("shows the Store, every Project's counts, the memory and the services", async () => {
    const w = await withRecords();
    const read = await w.api.storage();
    await renderApp(w, "/?view=usage&tab=storage");
    expect(figure("Store", "Kind")).toBe("In memory");
    expect(figure("Store", "Size")).toBe(bytes(read.size));
    // Each Ticket's Project has the scenario's default Spec, and Beta one more.
    expect(rows("Projects' records")).toEqual(["Acme | 1 | 1 | 0", "Beta | 2 | 2 | 0", "Old | 0 | 0 | 0"]);
    // The heap moves between reads, so only its form is fixed; WebAssembly
    // gives no resident size, so that row is left out.
    expect(figure("Memory", "Go heap")).toMatch(/^\d+\.\d MB$/);
    expect(within(storageTable("Memory")).queryByRole("row", { name: /^Resident/ })).toBeNull();
    expect(rows("Services")).toEqual([`laptop | ${read.services[0].version} | just now`]);
  });

  it("narrows the counts to the scoped Project without reading again, and keeps the memory and services whole", async () => {
    const w = await withRecords();
    const { api, reads } = counting(w.api);
    await renderApp(w, "/?view=usage&tab=storage", api);
    expect(reads()).toBe(1);
    await userEvent.selectOptions(scope(), "beta");
    await settle(world);
    expect(rows("Projects' records")).toEqual(["Beta | 2 | 2 | 0"]);
    expect(storageTable("Memory")).toBeInTheDocument();
    expect(rows("Services")).toHaveLength(1);
    expect(reads()).toBe(1);
  });

  it("loads when opened, not with the rest of Usage", async () => {
    const w = await withRecords();
    const { api, reads } = counting(w.api);
    await renderApp(w, "/?view=usage", api);
    expect(reads()).toBe(0);
    await openTab("Storage");
    expect(reads()).toBe(1);
    expect(tab("Storage")).toHaveAttribute("aria-selected", "true");
  });

  it("shows a Postgres Store by its host, port and database, with no user or password", async () => {
    const w = await withRecords();
    const postgres: Api = {
      ...w.api,
      storage: async (project) => {
        const v: StorageView = await w.api.storage(project);
        return { ...v, kind: "postgres", path: undefined, host: "db.example.com", port: 5432, database: "central" };
      },
    };
    await renderApp(w, "/?view=usage&tab=storage", postgres);
    expect(figure("Store", "Kind")).toBe("Postgres");
    expect(figure("Store", "Location")).toBe("db.example.com:5432/central");
    expect(usage().textContent).not.toMatch(/postgres:\/\/|@db\.example\.com/);
  });

  it("shows the read's failure", async () => {
    const w = await withRecords();
    const down: Api = {
      ...w.api,
      storage: async () => {
        throw new Error("The Store is down");
      },
    };
    await renderApp(w, "/?view=usage&tab=storage", down);
    expect(within(usage()).getByRole("alert")).toHaveTextContent("The Store is down");
    expect(screen.queryByRole("table", { name: "Store" })).toBeNull();
  });
});
