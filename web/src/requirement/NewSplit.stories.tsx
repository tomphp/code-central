// + New ▾, with its menu open, and the Small Change popup opened from it.
// On the WASM Core: the Projects acme and beta, as props. The api is
// passed because each popup submits.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { Api } from "../api/api";
import { storyWorld } from "../api/core/story";
import type { ProjectRow } from "../api/types";
import { NewSplit } from "./NewSplit";

interface World {
  api: Api;
  projects: ProjectRow[];
}

const meta = {
  title: "Requirement/NewSplit",
  ...storyWorld(async (s, w): Promise<World> => {
    await s.project("acme", "Acme");
    await s.project("beta", "Beta");
    return { api: w.api, projects: await w.api.listProjects() };
  }),
  // The menu is placed against the nearest positioned ancestor.
  render: (_, { loaded }) => (
    <div style={{ position: "relative", width: 260 }}>
      <NewSplit variant="primary" {...(loaded.world as World)} onSubmitted={() => {}} />
    </div>
  ),
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const Closed: Story = {};

export const MenuOpen: Story = {
  play: async ({ canvasElement }) => {
    await userEvent.click(within(canvasElement).getByRole("button", { name: "More to create" }));
  },
};

export const SmallChangeFromTheMenu: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByRole("button", { name: "More to create" }));
    await userEvent.click(canvas.getByRole("menuitem", { name: "Small Change" }));
  },
};
