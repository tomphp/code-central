import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Select } from "./Select";

const meta = {
  title: "Controls/Select",
  component: Select<string>,
} satisfies Meta<typeof Select<string>>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    "aria-label": "Effort",
    options: [{ value: "low" }, { value: "medium" }, { value: "high" }],
    value: "medium",
    onChange: () => {},
  },
  render: (args) => {
    const [value, setValue] = useState(args.value);
    return <Select {...args} value={value} onChange={setValue} />;
  },
};

export const WithPlaceholder: Story = {
  ...Default,
  args: { ...Default.args, placeholder: "Project's settings", value: "" },
};

export const Disabled: Story = {
  ...Default,
  args: { ...Default.args, disabled: true },
};
