import { useState } from "react";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ToggleGroup } from "./ToggleGroup";

const meta = { title: "Controls/ToggleGroup", component: ToggleGroup<string> } satisfies Meta<typeof ToggleGroup<string>>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    label: "Group by",
    options: [
      { value: "state", label: "State" },
      { value: "spec", label: "Spec" },
    ],
    value: "state",
    onChange: () => {},
  },
  render: (args) => {
    const [value, setValue] = useState(args.value);
    return <ToggleGroup {...args} value={value} onChange={setValue} />;
  },
};
