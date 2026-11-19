package ops

import (
	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/core"
)

// Chat (Spec code-central-633) and the Factory's runtime reports: its
// wake, Heartbeat, base, Reported Models and Model Choice.

// GetChat is get_chat.
var GetChat = &Definition[api.GetChatInput, api.ConversationView]{
	Name: "get_chat", Audience: HumanOnly,
	HTTP: "GET /api/p/{key}/chat",
	Doc:  "The Project's current Conversation.",
	Call: (*core.Core).GetChat,
}

// StartConversation is start_conversation.
var StartConversation = &Definition[api.StartConversationInput, api.ConversationView]{
	Name: "start_conversation", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/start_conversation",
	Doc:  "Start a new Conversation, replacing the current one.",
	Call: (*core.Core).StartConversation,
}

// SendChatMessage is send_chat_message.
var SendChatMessage = &Definition[api.SendChatMessageInput, api.ConversationView]{
	Name: "send_chat_message", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/send_chat_message",
	Doc:  "Send a message in the current Conversation.",
	Call: (*core.Core).SendChatMessage,
}

// StopChatReply is stop_chat_reply.
var StopChatReply = &Definition[api.StopChatReplyInput, api.ConversationView]{
	Name: "stop_chat_reply", Audience: HumanOnly, Write: true,
	HTTP: "POST /api/p/{key}/stop_chat_reply",
	Doc:  "Stop the running reply in the current Conversation.",
	Call: (*core.Core).StopChatReply,
}

// NextChatWork is next_chat_work, a long-poll.
var NextChatWork = &Definition[api.NextChatWorkInput, api.ChatWork]{
	Name: "next_chat_work", Audience: FactoryOnly,
	HTTP: "GET /api/p/{key}/chat/next-work",
	Doc:  "The Factory's long-poll for the next Conversation turn or stop; holds up to the wait timeout and returns empty when there is none.",
	Call: (*core.Core).NextChatWork,
}

// NextFactoryWake is next_factory_wake, a long-poll.
var NextFactoryWake = &Definition[api.NextFactoryWakeInput, api.FactoryWake]{
	Name: "next_factory_wake", Audience: FactoryOnly,
	HTTP: "GET /api/p/{key}/factory/next-wake",
	Doc:  "The Factory's long-poll that ends when a write its poll acts on reaches the Project; holds up to the wait timeout and returns empty when none does.",
	Call: (*core.Core).NextFactoryWake,
}

// ReportHeartbeat is report_heartbeat.
var ReportHeartbeat = &Definition[api.ReportHeartbeatInput, api.Written[api.ProjectView]]{
	Name: "report_heartbeat", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_heartbeat",
	Doc:  "The Factory's Heartbeat.",
	Call: (*core.Core).ReportHeartbeat,
}

// ReportBase is report_base.
var ReportBase = &Definition[api.ReportBaseInput, api.Written[api.ProjectView]]{
	Name: "report_base", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_base",
	Doc:  "The Factory's report of the state of its base under Pull Request.",
	Call: (*core.Core).ReportBase,
}

// ReportModels is report_models.
var ReportModels = &Definition[api.ReportModelsInput, api.Written[api.ProjectView]]{
	Name: "report_models", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/report_models",
	Doc:  "The Factory's report of what its claude accepts.",
	Call: (*core.Core).ReportModels,
}

// GetModelChoice is get_model_choice.
var GetModelChoice = &Definition[api.GetModelChoiceInput, api.ModelChoice]{
	Name: "get_model_choice", Audience: FactoryOnly,
	HTTP: "GET /api/p/{key}/tickets/{n}/model-choice",
	Doc:  "The Model Choice for a Ticket's next turn, which the Factory reads before every resume.",
	Call: (*core.Core).GetModelChoice,
}

// ReportModelChoice is report_model_choice.
var ReportModelChoice = &Definition[api.ReportModelChoiceInput, api.Written[api.TicketView]]{
	Name: "report_model_choice", Audience: FactoryOnly, Write: true,
	HTTP: "POST /api/p/{key}/tickets/{n}/report_model_choice",
	Doc:  "The Factory's report of what a turn was started with.",
	Call: (*core.Core).ReportModelChoice,
}
