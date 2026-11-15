import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "./Button";
import { PopupShell } from "./PopupShell";

const meta = {
  title: "Controls/PopupShell",
  component: PopupShell,
  args: { title: "New Map", onClose: () => {}, children: <p>What the popup holds.</p> },
} satisfies Meta<typeof PopupShell>;
export default meta;
type Story = StoryObj<typeof meta>;

// A form's popup: narrow, with its title.
export const Narrow: Story = {};

// With an actions row at its foot, a muted note leading it.
export const WithActions: Story = {
  args: {
    actions: (
      <>
        <span className="muted small">Creates a Map and its Charting Ticket.</span>
        <Button>Cancel</Button>
        <Button variant="primary">Submit map</Button>
      </>
    ),
  },
};

// A Board popup: wide, named by its label, with its pane's own heading and
// something else in the header row.
export const Wide: Story = {
  args: {
    title: undefined,
    label: "Focus popup",
    size: "wide",
    head: <span className="focus-left">3 left</span>,
    children: (
      <>
        <h2>acme-1 Add a sign-in page</h2>
        <p>What the popup holds.</p>
      </>
    ),
  },
};
