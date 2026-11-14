package store

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// legacyResponseKeys are the keys a pushed commit held the human's
// response under before code-central-432 stored it on the Project, each
// named as a ciResponseFile names it.
var legacyResponseKeys = []string{"land_anyway", "rerun", "reruns"}

// moveLegacyCIResponses is format 36 to 37 (code-central-585): each
// Land anyway and Re-run request stored on a Ticket's pushed commit
// moves into its Project's CI response for that commit, as reads
// overlaid them until then: Land anyway taken from either place, and the
// later of the two Re-run requests kept, with its attempts. A Ticket or
// Project with nothing to move is left as it was.
func moveLegacyCIResponses(stage string) error {
	projects, err := filepath.Glob(filepath.Join(stage, "projects", "*", "project.yaml"))
	if err != nil {
		return err
	}
	for _, path := range projects {
		if err := moveProjectLegacyCIResponses(path); err != nil {
			return err
		}
	}
	return nil
}

// moveProjectLegacyCIResponses moves the responses of the Tickets beside
// project.yaml at path into it, in Ticket order.
func moveProjectLegacyCIResponses(path string) error {
	tickets, err := filepath.Glob(filepath.Join(filepath.Dir(path), "tickets", "*.yaml"))
	if err != nil {
		return err
	}
	number := func(p string) int {
		n, _ := strconv.Atoi(strings.TrimSuffix(filepath.Base(p), ".yaml"))
		return n
	}
	slices.SortFunc(tickets, func(a, b string) int { return cmp.Compare(number(a), number(b)) })
	var moved []ciResponseFile
	for _, tp := range tickets {
		rs, err := stripLegacyResponses(tp)
		if err != nil {
			return fmt.Errorf("%s: %w", tp, err)
		}
		moved = append(moved, rs...)
	}
	if len(moved) == 0 {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var pf projectFile
	if field, err := decodeStrict(data, &pf); err != nil {
		return fmt.Errorf("%s: %s: %w", path, field, err)
	}
	for _, l := range moved {
		if pf.CIResponses, err = overlayLegacyResponse(pf.CIResponses, l); err != nil {
			return fmt.Errorf("%s: pushed commit %s: %w", path, l.Commit, err)
		}
	}
	out, err := encodeYAML(pf)
	if err != nil {
		return err
	}
	return atomicWrite(path, out)
}

// stripLegacyResponses removes every response from the pushed commits of
// the Ticket file at path, rewriting it only when it held one, and
// returns them in pushed order, each naming its commit.
func stripLegacyResponses(path string) ([]ciResponseFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, ok := parseBlockYAML(data)
	if !ok {
		doc = new(yaml.Node)
		if err := yaml.Unmarshal(data, doc); err != nil {
			return nil, fmt.Errorf("not valid YAML: %v", err)
		}
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	var out []ciResponseFile
	for i, pc := range mappingValue(doc.Content[0], "pushed_commits").Content {
		if pc.Kind != yaml.MappingNode {
			continue
		}
		l := ciResponseFile{Commit: mappingValue(pc, "commit").Value}
		var kept []*yaml.Node
		found := false
		for j := 0; j+1 < len(pc.Content); j += 2 {
			k, v := pc.Content[j], pc.Content[j+1]
			if !slices.Contains(legacyResponseKeys, k.Value) {
				kept = append(kept, k, v)
				continue
			}
			found = true
			if field, err := decodeNode(&yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{k, v}}, reflect.ValueOf(&l).Elem()); err != nil {
				return nil, fmt.Errorf("pushed_commits[%d].%s: %w", i, field, err)
			}
		}
		if found {
			pc.Content = kept
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	var tf ticketFile
	if field, err := decodeNode(doc.Content[0], reflect.ValueOf(&tf).Elem()); err != nil {
		return nil, fmt.Errorf("%s: %w", orFile(field), err)
	}
	enc, err := encodeYAML(tf)
	if err != nil {
		return nil, err
	}
	return out, atomicWrite(path, enc)
}

// mappingValue is the value under key in mapping n, or an empty node.
func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				return n.Content[i+1]
			}
		}
	}
	return &yaml.Node{}
}

// overlayLegacyResponse is rs with l folded into the response to its
// commit, one added when there is none: Land anyway kept from either,
// and the later Re-run request with its attempts, the pushed commit's on
// a tie, as reads did.
func overlayLegacyResponse(rs []ciResponseFile, l ciResponseFile) ([]ciResponseFile, error) {
	i := slices.IndexFunc(rs, func(r ciResponseFile) bool { return r.Commit == l.Commit })
	if i < 0 {
		rs = append(rs, ciResponseFile{Commit: l.Commit})
		i = len(rs) - 1
	}
	r := &rs[i]
	if r.LandAnyway == "" {
		r.LandAnyway = l.LandAnyway
	}
	if l.Rerun == "" {
		return rs, nil
	}
	pushed, err := parseTime(l.Rerun)
	if err != nil {
		return nil, fmt.Errorf("rerun: %w", err)
	}
	if r.Rerun != "" {
		project, err := parseTime(r.Rerun)
		if err != nil {
			return nil, fmt.Errorf("ci response rerun: %w", err)
		}
		if project.After(pushed) {
			return rs, nil
		}
	}
	r.Rerun, r.Reruns = l.Rerun, l.Reruns
	return rs, nil
}
