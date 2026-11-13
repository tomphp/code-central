import type { Meta, StoryObj } from "@storybook/react-vite";
import { OneProject, ShortId } from "./ShortId";

const meta = { title: "Inbox/ShortId", component: ShortId, args: { id: "code-central-492" } } satisfies Meta<typeof ShortId>;
export default meta;
type Story = StoryObj<typeof meta>;

// Several Projects selected: the full ID.
export const Full: Story = {};

// One Project selected: the number alone.
export const OneProjectSelected: Story = {
  decorators: [(Story) => <OneProject.Provider value={true}><Story /></OneProject.Provider>],
};
