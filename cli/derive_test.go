package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tomoram/code-central/api"
	"github.com/tomoram/code-central/client"
	"github.com/tomoram/code-central/ops"
)

// deriveInput has one field of each shape a derived command takes.
type deriveInput struct {
	Target api.Ref           `json:"target" required:"true" cli:"<id>"`
	Others []api.Ref         `json:"others" required:"true" cli:"<other>..."`
	Commit string            `json:"commit" required:"true" cli:"--commit <sha>"`
	Name   string            `json:"name,omitempty" cli:"--name <n>"`
	Labels []string          `json:"labels,omitempty" cli:"--label <l>"`
	Tags   map[string]string `json:"tags,omitempty" cli:"--tag <key>=<value>"`
	Full   bool              `json:"full,omitempty"`
	Secret string            `json:"-"`
}

type deriveResult struct {
	Got deriveInput `json:"got"`
}

var deriveDef = &ops.Definition[deriveInput, deriveResult]{
	Name: "do_thing", Audience: ops.AgentAndHuman,
	HTTP: "POST /api/p/{key}/do_thing",
	Doc:  "Do a thing.",
}

// runDerived runs c with args under Project acme, recording what it sent.
func runDerived(t *testing.T, c command, args ...string) (stdout string, err error) {
	t.Helper()
	var out bytes.Buffer
	e := Env{
		Stdout: &out, Stderr: &bytes.Buffer{},
		Getenv: func(k string) string {
			return map[string]string{"CODE_CENTRAL_PROJECT": "acme", "CODE_CENTRAL_URL": "http://127.0.0.1:1"}[k]
		},
		Dir: t.TempDir(),
	}
	err = c.run(context.Background(), e, args)
	return out.String(), err
}

func echo(_ *client.Client, _ context.Context, key string, in deriveInput) (deriveResult, error) {
	if key != "acme" {
		return deriveResult{}, &api.Error{Code: api.UnknownProject, Message: key}
	}
	return deriveResult{Got: in}, nil
}

func TestADerivedCommandsUsageFollowsItsInputsTags(t *testing.T) {
	c := derive(deriveDef, echo, "")
	want := "do-thing <id> <other>... --commit <sha> [--name <n>] [--label <l>]... [--tag <key>=<value>]... [--full]"
	if c.usage != want {
		t.Fatalf("usage\n %s\nwant\n %s", c.usage, want)
	}
	if c.op != "do_thing" || c.help != "Do a thing." {
		t.Fatalf("op %q help %q, want the definition's", c.op, c.help)
	}
	if noted := derive(deriveDef, echo, "does it"); noted.usage != want+"  does it" {
		t.Fatalf("usage with a note: %s", noted.usage)
	}
}

func TestADerivedCommandFillsItsInputFromTheArguments(t *testing.T) {
	out, err := runDerived(t, derive(deriveDef, echo, ""),
		"x-1", "x-2", "--commit", "abc", "x-3", "--label", "a", "--label", "b", "--tag", "k=v", "--full")
	if err != nil {
		t.Fatal(err)
	}
	var got deriveResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	want := deriveInput{
		Target: "x-1", Others: []api.Ref{"x-2", "x-3"}, Commit: "abc",
		Labels: []string{"a", "b"}, Tags: map[string]string{"k": "v"}, Full: true,
	}
	if !reflect.DeepEqual(got.Got, want) {
		t.Fatalf("sent %+v, want %+v", got.Got, want)
	}
}

func TestADerivedCommandLeavesUnsetListsNil(t *testing.T) {
	type in struct {
		Labels []string          `json:"labels,omitempty" cli:"--label <l>"`
		Tags   map[string]string `json:"tags,omitempty" cli:"--tag <key>=<value>"`
	}
	def := &ops.Definition[in, in]{Name: "find_things"}
	var sent in
	c := derive(def, func(_ *client.Client, _ context.Context, _ string, i in) (in, error) { sent = i; return i, nil }, "")
	if _, err := runDerived(t, c); err != nil {
		t.Fatal(err)
	}
	if sent.Labels != nil || sent.Tags != nil {
		t.Fatalf("sent %#v, want nil lists", sent)
	}
	if _, err := runDerived(t, c, "stray"); err == nil || err.Error() != `unexpected argument "stray"` {
		t.Fatalf("a stray argument: %v", err)
	}
}

func TestADerivedCommandRefusesBadArgumentsAsUsageErrors(t *testing.T) {
	c := derive(deriveDef, echo, "")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{nil, "expected a Spec or Ticket ID"},
		{[]string{"x-1", "--commit", "abc"}, "expected at least 1 argument(s) after the ID"},
		{[]string{"x-1", "x-2", "--tag", "nokey"}, `a Tag is <key>=<value>, not "nokey"`},
		{[]string{"x-1", "x-2", "--bogus"}, "flag provided but not defined: -bogus"},
	} {
		_, err := runDerived(t, c, tc.args...)
		var ue usageError
		if !errors.As(err, &ue) || ue.msg != tc.want {
			t.Errorf("%v: %v, want the usage error %q", tc.args, err, tc.want)
		}
	}
}

func TestADerivedCommandWithASecondPositionalTakesExactlyOne(t *testing.T) {
	type in struct {
		Ticket  api.Ref `json:"ticket" required:"true" cli:"<id>"`
		Blocker api.Ref `json:"blocker" required:"true" cli:"<blocker-id>"`
	}
	def := &ops.Definition[in, in]{Name: "link"}
	c := derive(def, func(_ *client.Client, _ context.Context, _ string, i in) (in, error) { return i, nil }, "")
	if c.usage != "link <id> <blocker-id>" {
		t.Fatalf("usage %q", c.usage)
	}
	if _, err := runDerived(t, c, "1", "2", "3"); err == nil || err.Error() != `unexpected argument "3"` {
		t.Fatalf("a third argument: %v", err)
	}
	out, err := runDerived(t, c, "1", "2")
	if err != nil || !strings.Contains(out, `"blocker": "2"`) {
		t.Fatalf("%v: %s", err, out)
	}
}

func TestAFieldTheCLICantTakeFailsAtStartUpNamingIt(t *testing.T) {
	type number struct {
		Count int `json:"count" cli:"--count <n>"`
	}
	type untagged struct {
		Body string `json:"body"`
	}
	type positionalBool struct {
		Flag bool `json:"flag" cli:"<flag>"`
	}
	for field, derived := range map[string]func(){
		"Count": func() { derive(&ops.Definition[number, number]{Name: "bad_op"}, nil, "") },
		"Body":  func() { derive(&ops.Definition[untagged, untagged]{Name: "bad_op"}, nil, "") },
		"Flag":  func() { derive(&ops.Definition[positionalBool, positionalBool]{Name: "bad_op"}, nil, "") },
	} {
		t.Run(field, func(t *testing.T) {
			defer func() {
				msg, _ := recover().(string)
				if !strings.Contains(msg, "bad_op") || !strings.Contains(msg, field) {
					t.Fatalf("panic %q, want one naming bad_op and %s", msg, field)
				}
			}()
			derived()
		})
	}
}

func TestEveryOperationCommandTakesItsHelpFromItsDefinition(t *testing.T) {
	for name, c := range commands {
		if c.op == "" {
			continue
		}
		def, ok := ops.DefinitionOf(c.op)
		if !ok {
			t.Errorf("%s: no definition for operation %q", name, c.op)
			continue
		}
		if c.help != def.Description() {
			t.Errorf("%s: help %q, want its definition's %q", name, c.help, def.Description())
		}
	}
}
