import type { Meta, StoryObj } from "@storybook/react-vite";
import { STATES } from "../api/types";
import { KindBadge } from "./KindBadge";
import { StageBadge } from "./StageBadge";
import { StateChip } from "./StateChip";

const meta = { title: "Controls/Badges" } satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const States: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
      {STATES.map((s) => (
        <StateChip key={s} state={s} />
      ))}
    </div>
  ),
};

export const Kinds: Story = {
  render: () => (
    <div style={{ display: "flex", gap: 8 }}>
      {["Requirement", "Map", "Long-lived", "Spec"].map((k) => (
        <KindBadge key={k} kind={k} />
      ))}
    </div>
  ),
};

export const Stage: Story = { render: () => <StageBadge label="Implementation" /> };
