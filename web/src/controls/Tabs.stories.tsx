import { useState, type ComponentProps } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Tabs } from "./Tabs";

const meta = { title: "Controls/Tabs", component: Tabs<string> } satisfies Meta<typeof Tabs<string>>;
export default meta;
type Story = StoryObj<typeof meta>;

// StatefulTabs starts on its value arg and changes as the Tabs do.
function StatefulTabs(args: ComponentProps<typeof Tabs<string>>) {
  const [value, setValue] = useState(args.value);
  return (
    <Tabs {...args} value={value} onChange={setValue}>
      <p>{args.tabs.find((t) => t.value === value)?.label} panel</p>
    </Tabs>
  );
}

export const Default: Story = {
  args: {
    label: "Usage",
    tabs: [
      { value: "tokens", label: "Token usage" },
      { value: "context", label: "Context Size" },
      { value: "calls", label: "Code Central calls" },
    ],
    value: "tokens",
    onChange: () => {},
  },
  render: (args) => <StatefulTabs {...args} />,
};
