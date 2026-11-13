import type { Meta, StoryObj } from "@storybook/react-vite";
import type { LinkView } from "../api/types";
import { LinkBadge } from "./LinkBadge";

const link: LinkView = {
  integration: "github",
  id: "#123",
  url: "https://github.com/acme/widgets/issues/123",
  external_state: "open",
  detached: false,
  outbound: false,
};

const meta = { title: "Ticket/LinkBadge", component: LinkBadge, args: { link } } satisfies Meta<typeof LinkBadge>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Linked: Story = {};
export const PlainOnARow: Story = { args: { plain: true } };
export const Outbound: Story = { args: { link: { ...link, outbound: true } } };
export const Unlinked: Story = { args: { link: { ...link, detached: true } } };
