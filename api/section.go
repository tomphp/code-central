package api

// Heading is one heading of a body's outline.
type Heading struct {
	Text  string `json:"text"`
	Level int    `json:"level"`
	// Size is the bytes of the Section the heading opens, subsections
	// included, so a reader can weigh a Section before fetching it.
	Size int `json:"size"`
}
