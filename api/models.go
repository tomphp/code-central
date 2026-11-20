package api

// ModelsView is a Project's Models as every Project read returns them:
// the Project default row, the Chat row, then one row for every kind of
// work, in GuideNames order, each empty where it sets nothing. The Chat
// row is not a kind of work (Spec code-central-633): Chat doesn't use the
// default row, so a costly default doesn't make Chat costly.
type ModelsView struct {
	Default ModelRowView     `json:"default"`
	Chat    ModelRowView     `json:"chat"`
	Kinds   []KindModelsView `json:"kinds"`
}

// ModelRowView is one Models row's model and effort, each empty when the
// row leaves it to the tier below.
type ModelRowView struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
}

// KindModelsView is the Models row for one kind of work, a guide name.
type KindModelsView struct {
	Kind string `json:"kind"`
	ModelRowView
}

// SetModelsInput is the human's set_models(default, chat, kinds, full?).
type SetModelsInput struct {
	Default ModelRowView `json:"default"`
	// Chat is the Chat row's model and effort.
	Chat ModelRowView `json:"chat"`
	// Kinds are the rows for kinds of work; a kind left out, or a row
	// setting nothing, sets nothing.
	Kinds []KindModelsView `json:"kinds"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
