import type { Meta, StoryObj } from "@storybook/react-vite";
import { CallsTable, DEFAULT_SORT } from "./CallsTable";
import { KindsTable } from "./KindsTable";
import { SessionsTable } from "./SessionsTable";
import { TokenChart } from "./TokenChart";
import { populated } from "./fixtures";

const names: Record<string, string> = { acme: "Acme", globex: "Globex", initech: "Initech" };

const meta = {
  title: "Usage/Parts",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const CallsSorted: Story = {
  render: () => <CallsTable operations={populated.operations} sort={DEFAULT_SORT} onSort={() => {}} />,
};

export const ContextSizeByKind: Story = {
  render: () => <KindsTable kinds={populated.kinds} />,
};

// The address names the Subagent, so its parent starts expanded.
export const RecentSessionsSubagentExpanded: Story = {
  render: () => (
    <SessionsTable
      sessions={populated.sessions}
      here={{ view: "usage", session: { project: "acme", session: "s1", agent: "a1" } }}
      projectName={(key) => names[key] ?? key}
      navigate={() => {}}
    />
  ),
};

export const Tokens: Story = {
  render: () => <TokenChart days={populated.days} kinds={populated.kinds.map((k) => k.kind)} />,
};
