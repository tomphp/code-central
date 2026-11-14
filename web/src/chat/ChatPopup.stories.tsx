// The Chat pop-up in each state it shows: a Conversation handed in as props,
// so the stories need no Core.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { claude, conversation, human, item, noConversation } from "./chatFixtures";
import { ChatPopup } from "./ChatPopup";

const meta = {
  title: "Chat/ChatPopup",
  component: ChatPopup,
  args: {
    project: "Acme",
    chatAvailable: true,
    chatRow: { model: "", effort: "" },
    reported: { effort_levels: ["low", "medium", "high"], aliases: ["haiku", "sonnet", "opus"], model_ids: [], model_flag: "", effort_flag: "" },
    conversation: conversation([]),
    onSend: () => {},
    onStop: () => {},
    onStart: () => {},
    onClose: () => {},
  },
  // The pop-up is fixed to the lower right; a transform makes the frame its container.
  decorators: [(Story) => <div style={{ position: "relative", height: 600, transform: "translateZ(0)" }}>{Story()}</div>],
} satisfies Meta<typeof ChatPopup>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {};

export const NoConversationYet: Story = { args: { conversation: noConversation() } };

export const Replying: Story = {
  args: {
    conversation: conversation(
      [human("What does the Factory do on a Heartbeat?"), claude([item("text", "It reports its **status** and whether it has Chat support")])],
      { running: true },
    ),
  },
};

export const WorkingOnIt: Story = { args: { conversation: conversation([human("Summarise the repo")], { running: true }) } };

export const ToolCalls: Story = {
  args: {
    conversation: conversation(
      [
        human("What is in the repo?"),
        claude(
          [
            item("tool_call", "Bash: ls", { tool: "Bash", tool_use_id: "t1" }),
            item("tool_result", "README.md\ncore\nfactory", { tool: "Bash", tool_use_id: "t1" }),
            item("tool_call", "Read: missing.md", { tool: "Read", tool_use_id: "t2" }),
            item("tool_result", "file not found", { tool: "Read", tool_use_id: "t2", is_error: true }),
            item("text", "The repo has a **README** and two packages:\n\n- `core`\n- `factory`"),
          ],
          { outcome: "done" },
        ),
      ],
      { resume_command: "cd /repo.worktrees/chat && claude --resume 1b2c" },
    ),
  },
};

export const PermissionDenied: Story = {
  args: {
    conversation: conversation([human("Delete it"), claude([item("permission_denials", "Bash(rm -rf build)"), item("text", "I wasn't allowed to do that.")], { outcome: "done" })]),
  },
};

export const Stopped: Story = {
  args: { conversation: conversation([human("Write an essay"), claude([item("text", "Once upon a")], { outcome: "stopped" })]) },
};

export const Failed: Story = {
  args: { conversation: conversation([human("Hi"), claude([], { outcome: "failed", error: "the model 'gpt-9' was rejected by claude" })]) },
};

export const OlderMessagesDropped: Story = {
  args: { conversation: conversation([human("The last question"), claude([item("text", "The last answer")], { outcome: "done" })], { tail: true }) },
};

export const FactoryNotOnline: Story = {
  args: {
    chatAvailable: false,
    conversation: conversation([human("Hi"), claude([item("text", "Hello")], { outcome: "done" })], { availability: "unavailable", factory: { name: "code-central-factory", host: "laptop" } }),
  },
};

export const NoFactoryWithChat: Story = {
  args: { chatAvailable: false, conversation: noConversation() },
};

export const Loading: Story = { args: { conversation: undefined } };

export const ModelAndEffort: Story = { args: { conversation: conversation([], { model: "opus", effort: "high" }) } };
