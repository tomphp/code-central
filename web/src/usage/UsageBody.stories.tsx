import type { Meta, StoryObj } from "@storybook/react-vite";
import { DEFAULT_SORT, UsageBody } from "./UsageBody";
import { empty, manyProjects, populated } from "./fixtures";

const names: Record<string, string> = { acme: "Acme", globex: "Globex", initech: "Initech" };

const meta = {
  title: "Usage/UsageBody",
  component: UsageBody,
  parameters: { layout: "fullscreen" },
  args: {
    days: 7,
    scopeName: "All Projects",
    here: { view: "usage" },
    projectName: (key: string) => names[key] ?? key,
    navigate: () => {},
    sort: DEFAULT_SORT,
    onSort: () => {},
  },
  render: (args) => (
    <main className="usage view-shell">
      <UsageBody {...args} />
    </main>
  ),
} satisfies Meta<typeof UsageBody>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Loading: Story = {};
export const Empty: Story = { args: { data: empty } };
export const Populated: Story = { args: { data: populated } };
export const ErrorState: Story = { args: { error: "usage: the store is unreadable" } };
export const OneProject: Story = { args: { data: populated, scopeName: "Acme" } };
export const ManyProjects: Story = { args: { data: manyProjects } };
