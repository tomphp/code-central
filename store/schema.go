package store

import (
	"fmt"
	"reflect"
	"strings"
)

// Schema describes, as plain text, everything the strict decoder accepts
// in each kind of file: every key with its type and whether it may be
// left out, and the closed sets of values load checks. core's snapshot
// test compares it against the checked-in snapshot for Format, so a
// schema change can't ship without a format bump. The Import manifest is
// left out: its shape is frozen and never versioned.
func Schema() string {
	var b strings.Builder
	file := func(name string, v any) {
		fmt.Fprintf(&b, "%s\n", name)
		describe(&b, reflect.TypeOf(v), "  ")
		b.WriteString("\n")
	}
	file("store.yaml", storeFile{})
	file("projects/<key>/project.yaml", projectFile{})
	file("projects/<key>/specs/<n>.md: YAML frontmatter between --- lines, then the Markdown body", specFront{})
	file("projects/<key>/tickets/<n>.yaml", ticketFile{})

	values := func(name string, vs []string) {
		fmt.Fprintf(&b, "%s: %s\n", name, strings.Join(vs, ", "))
	}
	b.WriteString("values\n")
	var states []string
	for _, s := range States {
		states = append(states, string(s))
	}
	values("  ticket state, moves[].from, moves[].to", states)
	values("  rounds[].questions[].status", []string{string(QuestionOpen), string(QuestionAnswered), string(QuestionDismissed)})
	var external []string
	for _, x := range ExternalStates {
		external = append(external, string(x))
	}
	values("  link.external_state", external)
	values("  spec edits[].previous keys", specEditFields)
	values("  ticket edits[].previous keys", ticketEditFields)
	return b.String()
}

func describe(b *strings.Builder, t reflect.Type, indent string) {
	for i := range t.NumField() {
		f := t.Field(i)
		name, opts, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		optional := ""
		if strings.Contains(opts, "omitempty") {
			optional = " (optional)"
		}
		if f.Type.Kind() == reflect.Slice && f.Type.Elem().Kind() == reflect.Struct {
			fmt.Fprintf(b, "%s%s: list of mappings%s\n", indent, name, optional)
			describe(b, f.Type.Elem(), indent+"    ")
			continue
		}
		if f.Type.Kind() == reflect.Pointer && f.Type.Elem().Kind() == reflect.Struct {
			fmt.Fprintf(b, "%s%s: mapping%s\n", indent, name, optional)
			describe(b, f.Type.Elem(), indent+"    ")
			continue
		}
		fmt.Fprintf(b, "%s%s: %s%s\n", indent, name, f.Type, optional)
	}
}
