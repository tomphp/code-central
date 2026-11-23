package domain

import "fmt"

// DisplayID is the ID shown everywhere and used as the branch name.
func DisplayID(key string, n int) string { return fmt.Sprintf("%s-%d", key, n) }
