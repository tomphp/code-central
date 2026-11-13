import type { Meta, StoryObj } from "@storybook/react-vite";
import { Field } from "./Field";
import { Select } from "./Select";

const meta = { title: "Controls/Field", component: Field } satisfies Meta<
  typeof Field
>;
export default meta;
type Story = StoryObj<typeof meta>;

export const WithHint: Story = {
  args: {
    label: "CI Provider",
    hint: "Where CI runs.",
    hintId: "ci-hint",
    children: (
      <Select
        options={[{ value: "github", label: "GitHub Actions" }]}
        value="github"
        onChange={() => {}}
        aria-describedby="ci-hint"
      />
    ),
  },
};

export const NoHint: Story = {
  args: { label: "Branch", children: <input defaultValue="main" /> },
};
