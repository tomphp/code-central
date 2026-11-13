import type { Meta, StoryObj } from "@storybook/react-vite";
import { ApiError } from "../api/types";
import { Refusal } from "./Refusal";

const meta = { title: "Options/Refusal", component: Refusal } satisfies Meta<
  typeof Refusal
>;
export default meta;
type Story = StoryObj<typeof meta>;

export const MessageOnly: Story = {
  args: {
    id: "refusal",
    error: new ApiError(422, "invalid", "The repository isn't owner/name."),
  },
};
export const WithFix: Story = {
  args: {
    id: "refusal",
    error: new ApiError(
      422,
      "ci_check_failed",
      "The check couldn't reach acme/web.",
      undefined,
      undefined,
      "Run gh auth login, then try again.",
    ),
  },
};
