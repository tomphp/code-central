package store

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
)

// LongLivedLabel marks a Long-lived Spec, one that never finishes, such
// as a CI Rule's Spec or a Project's Issues Spec.
const LongLivedLabel = "long-lived"

// labelLongLivedSpecs is format 24 to 25: each CI Rule's Spec and each
// Project's Issues Spec gets the long-lived Label. It runs once, with
// the format bump, so a Label the human later removes isn't added back.
func labelLongLivedSpecs(stage string) error {
	projects, err := filepath.Glob(filepath.Join(stage, "projects", "*", "project.yaml"))
	if err != nil {
		return err
	}
	for _, path := range projects {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var pf projectFile
		if _, err := decodeStrict(data, &pf); err != nil {
			return err
		}
		var specs []int
		if pf.CI != nil {
			for _, r := range pf.CI.Rules {
				specs = append(specs, r.Spec)
			}
		}
		if pf.GitHubIssues != nil && pf.GitHubIssues.Spec != 0 {
			specs = append(specs, pf.GitHubIssues.Spec)
		}
		for _, n := range specs {
			if err := addSpecLabel(filepath.Join(filepath.Dir(path), "specs", strconv.Itoa(n)+".md"), LongLivedLabel); err != nil {
				return err
			}
		}
	}
	return nil
}

// addSpecLabel adds label to the Spec file at path, if it exists and
// lacks it.
func addSpecLabel(path, label string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	front, body, ok := splitFrontmatter(data)
	if !ok {
		return nil
	}
	var f specFront
	if _, err := decodeStrict(front, &f); err != nil {
		return err
	}
	if slices.Contains(f.Labels, label) {
		return nil
	}
	f.Labels = append(f.Labels, label)
	out, err := encodeYAML(f)
	if err != nil {
		return err
	}
	return atomicWrite(path, append(append(append([]byte("---\n"), out...), "---\n"...), body...))
}
