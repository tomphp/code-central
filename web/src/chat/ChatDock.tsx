// Chat in the lower right corner of a Project-scoped view (Spec
// code-central-633): the floating icon, shown only when Chat is available
// for the Project, and the pop-up it opens. The pop-up follows the
// Conversation and, once open, stays open if Chat stops being available.
import { useState } from "react";
import type { ApiError, ModelRow, ProjectRow } from "../api/types";
import { Button } from "../controls/Button";
import { useApi } from "../live";
import "./chat.css";
import { ChatPopup } from "./ChatPopup";
import { useConversation } from "./useConversation";

// chatAvailable is whether Chat is available for the Project: at least
// one of its Factories is Online and reports Chat support.
export const chatAvailable = (project: ProjectRow) => project.factory_status.factories.some((f) => f.status === "online" && f.chat);

export interface ChatDockProps {
  // project is the Project the view is scoped to; none in the all-Projects scope.
  project?: ProjectRow;
}

// ChatDock is the icon and pop-up for the view's Project, or nothing in the
// all-Projects scope.
export function ChatDock({ project }: ChatDockProps) {
  // Keyed by Project, so a pop-up never carries over to another one.
  return project ? <Dock key={project.key} project={project} /> : null;
}

// Dock holds the open state and the Conversation for one Project.
function Dock({ project }: { project: ProjectRow }) {
  const api = useApi();
  const [open, setOpen] = useState(false);
  const [error, setError] = useState<ApiError>();
  const conversation = useConversation(project.key, open);
  const available = chatAvailable(project);

  // act runs one Chat write; the stream shows its effects, a refusal shows
  // here and rejects the returned promise.
  const act = (write: () => Promise<unknown>) => {
    setError(undefined);
    return write().catch((e: ApiError) => {
      setError(e);
      throw e;
    });
  };

  if (!open) {
    if (!available) return null;
    return (
      <Button type="button" className="chat-icon" aria-label="Chat" onClick={() => setOpen(true)}>
        <svg aria-hidden="true" viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinejoin="round">
          <path d="M21 12a8 8 0 0 1-11.6 7.1L4 20l1-4.6A8 8 0 1 1 21 12z" />
        </svg>
      </Button>
    );
  }
  return (
    <ChatPopup
      project={project.display_name || project.key}
      conversation={conversation}
      chatAvailable={available}
      chatRow={project.models.chat}
      reported={project.reported_models}
      error={error}
      onSend={(text) => act(() => api.sendChatMessage(project.key, text))}
      onStop={() => void act(() => api.stopChatReply(project.key)).catch(() => {})}
      onStart={(pick: ModelRow) => void act(() => api.startConversation(project.key, pick)).catch(() => {})}
      onClose={() => {
        setError(undefined);
        setOpen(false);
      }}
    />
  );
}
