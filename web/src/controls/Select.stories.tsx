import { useState, type ComponentProps } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Select } from "./Select";

const meta = {
  title: "Controls/Select",
  component: Select<string>,
} satisfies Meta<typeof Select<string>>;
export default meta;
type Story = StoryObj<typeof meta>;

// StatefulSelect starts on its value arg and changes as the Select does.
function StatefulSelect(args: ComponentProps<typeof Select<string>>) {
  const [value, setValue] = useState(args.value);
  return <Select {...args} value={value} onChange={setValue} />;
}

export const Default: Story = {
  args: {
    "aria-label": "Effort",
    options: [{ value: "low" }, { value: "medium" }, { value: "high" }],
    value: "medium",
    onChange: () => {},
  },
  render: (args) => <StatefulSelect {...args} />,
};

export const WithPlaceholder: Story = {
  ...Default,
  args: { ...Default.args, placeholder: "Project's settings", value: "" },
};

export const Disabled: Story = {
  ...Default,
  args: { ...Default.args, disabled: true },
};
