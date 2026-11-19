package api

// Who says which Actor may take an Action.
type Who string

// The Who values of the action table.
const (
	// AnyActor is "Human or agent".
	AnyActor Who = "human or agent"
	// AgentActor is "Agent". The human can do everything agents can, so
	// any Actor may take these.
	AgentActor Who = "agent"
	// Holder is "Agent (holder)": the agent must send the live Claim ID.
	Holder Who = "agent (holder)"
	// HumanActor is "Human": only the human Actor.
	HumanActor Who = "human"
	// Automatic moves are made by core itself, never asked for.
	Automatic Who = "automatic"
	// FactoryMove moves are the Factory's own, made through a FactoryOnly
	// operation: no Actor takes them as an action, so none is offered
	// them.
	FactoryMove Who = "factory"
)
