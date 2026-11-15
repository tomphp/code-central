// The Board's Spec popup, with the Spec pane and with the Requirement pane.
// On the WASM Core: acme's Spec "Sign-in" with two Tickets, and a
// Requirement standing in Build, read back and handed in as props. The api
// is passed because the panes act.
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { Api } from "../api/api";
import { storyWorld } from "../api/core/story";
import { ApiError, type ProjectRow, type SpecDetail } from "../api/types";
import type { Link } from "../url";
import { SpecPopupView } from "./SpecPopup";

interface World {
  api: Api;
  now: Date;
  projects: ProjectRow[];
  spec: SpecDetail;
  requirement: SpecDetail;
}

const meta = {
  title: "Board/SpecPopup",
  ...storyWorld(async (s, w): Promise<World> => {
    await s.project("acme", "Acme");
    const spec = await s.spec("acme", { title: "Sign-in", body: "## Problem Statement\n\nThere is no way to sign in." });
    await s.ticket("acme", { title: "Add a sign-in page", spec });
    await s.ticket("acme", { title: "Rate-limit the sign-in endpoint", spec, state: "In Progress" });
    const requirement = await s.requirement("acme", "Search across Tickets and Specs\n\nWith filters that remember what you last picked.");
    return {
      api: w.api,
      now: w.now(),
      projects: await w.api.listProjects(),
      spec: await w.api.getSpec("acme", spec),
      requirement: await w.api.getSpec("acme", requirement),
    };
  }),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

const nowhere = (id: string): Link => ({ href: `#${id}`, onClick: (e) => e.preventDefault() });

const popup = (pick: (w: World) => SpecDetail, read: (w: World) => { data?: SpecDetail; error?: ApiError }): Story => ({
  render: (_, { loaded }) => {
    const w = loaded.world as World;
    return <SpecPopupView api={w.api} now={w.now} id={pick(w).id} projects={w.projects} spec={read(w)} onClose={() => {}} open={() => {}} specLink={nowhere} />;
  },
});

export const Spec = popup(
  (w) => w.spec,
  (w) => ({ data: w.spec }),
);
export const Requirement = popup(
  (w) => w.requirement,
  (w) => ({ data: w.requirement }),
);
export const Loading = popup(
  (w) => w.spec,
  () => ({}),
);
export const ReadFailed = popup(
  (w) => w.spec,
  () => ({ error: new ApiError(404, "not_found", "no Spec acme-1") }),
);
