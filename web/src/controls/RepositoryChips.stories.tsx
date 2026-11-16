import type { Meta, StoryObj } from "@storybook/react-vite";
import { RepositoryChips } from "./RepositoryChips";

const meta = { title: "Controls/RepositoryChips", component: RepositoryChips } satisfies Meta<typeof RepositoryChips>;
export default meta;
type Story = StoryObj<typeof meta>;

// A Ticket that changes two Repositories.
export const Several: Story = { args: { names: ["acme", "api"] } };

// A Ticket that changes one.
export const One: Story = { args: { names: ["web"] } };
