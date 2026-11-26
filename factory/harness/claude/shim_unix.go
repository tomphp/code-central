//go:build unix

package claude

// refuseShim refuses a claude that can't take the Factory's prompts: any
// claude will do on Unix.
func refuseShim(string) error { return nil }
