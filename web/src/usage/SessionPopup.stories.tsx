import type { Meta, StoryObj } from "@storybook/react-vite";
import { SessionPopupBody } from "./SessionPopup";
import { sessionResponses } from "./fixtures";

const meta = {
  title: "Usage/SessionPopup",
  component: SessionPopupBody,
  args: {
    session: { project: "acme", session: "s1" },
    responses: sessionResponses,
    here: { view: "usage", tab: "context" },
    navigate: () => {},
    onClose: () => {},
  },
} satisfies Meta<typeof SessionPopupBody>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Populated: Story = {};

// A Subagent's, named by its Agent Type and description, with the link to
// its parent session.
export const Subagent: Story = {
  args: {
    session: { project: "acme", session: "s1", agent: "a1" },
    subagent: "Explore · Map Usage",
    responses: sessionResponses.slice(0, 3),
  },
};

export const NoResponses: Story = { args: { session: { project: "acme", session: "s-gone" }, responses: [] } };
export const Loading: Story = { args: { responses: undefined } };
export const ErrorState: Story = { args: { responses: undefined, error: "usage: the store is unreadable" } };
