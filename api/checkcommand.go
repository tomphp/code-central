package api

// SetCheckCommandInput is the human's set_check_command(check_command?,
// full?): the Project's Check Command, left out or empty for none.
type SetCheckCommandInput struct {
	CheckCommand string `json:"check_command,omitempty"`
	// Full asks for the Project in place of the Acknowledgement.
	Full bool `json:"full,omitempty"`
}
