package core_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tomoram/code-central/core"
)

type decodeTarget struct {
	Title string `json:"title"`
	Count int    `json:"count"`
}

func decodeErr(t *testing.T, ctx context.Context, in string) *core.Error {
	t.Helper()
	var v decodeTarget
	err := core.Decode(ctx, []byte(in), &v)
	var e *core.Error
	if !errors.As(err, &e) || e.Code != core.Invalid {
		t.Fatalf("decode %s: %v, want invalid", in, err)
	}
	return e
}

const bareUnknown = `invalid JSON: json: unknown field "working_dir": accepted fields are title, count`

func TestAnUnknownFieldListsTheAcceptedFieldsAndNamesTheLikelyOne(t *testing.T) {
	e := decodeErr(t, context.Background(), `{"titel":"T"}`)
	want := `invalid JSON: json: unknown field "titel": accepted fields are title, count; did you mean ` + "`title`?"
	if e.Message != want || e.Field != "titel" {
		t.Fatalf("got %q field %q, want %q", e.Message, e.Field, want)
	}
	e = decodeErr(t, context.Background(), `{"title_id":"T"}`)
	if !strings.Contains(e.Message, "did you mean `title`?") {
		t.Fatalf("got %q, want a suggestion of title", e.Message)
	}
}

func TestAnUnknownFieldNamesBothVersionsWhenTheClientsDiffers(t *testing.T) {
	ctx := core.WithVersions(context.Background(), "v1.8", "v1.9")
	e := decodeErr(t, ctx, `{"title":"T","working_dir":"/w"}`)
	want := bareUnknown + ": the caller is version v1.9 and the service is v1.8; run `code-central service restart`"
	if e.Message != want || e.Field != "working_dir" {
		t.Fatalf("got %q field %q, want %q", e.Message, e.Field, want)
	}
}

func TestAnUnknownFieldGivesASoftHintWhenTheClientsVersionIsUnknown(t *testing.T) {
	ctx := core.WithVersions(context.Background(), "v1.8", "")
	e := decodeErr(t, ctx, `{"working_dir":"/w"}`)
	want := bareUnknown + " (the service is v1.8; if your client is newer, run `code-central service restart`)"
	if e.Message != want || e.Field != "working_dir" {
		t.Fatalf("got %q field %q, want %q", e.Message, e.Field, want)
	}
}

func TestAnUnknownFieldIsUnchangedWhenTheVersionsMatchOrTheServicesIsUnknown(t *testing.T) {
	for name, ctx := range map[string]context.Context{
		"same version":       core.WithVersions(context.Background(), "v1.8", "v1.8"),
		"no service version": core.WithVersions(context.Background(), "", "v1.9"),
		"no versions":        context.Background(),
	} {
		t.Run(name, func(t *testing.T) {
			e := decodeErr(t, ctx, `{"working_dir":"/w"}`)
			if e.Message != bareUnknown || e.Field != "working_dir" {
				t.Fatalf("got %q field %q", e.Message, e.Field)
			}
		})
	}
}

func TestClientVersionIsStrippedAndWinsOverTheHeader(t *testing.T) {
	ctx := core.WithVersions(context.Background(), "v1.8", "v1.8")
	var v decodeTarget
	if err := core.Decode(ctx, []byte(`{"title":"T","client_version":"v1.9","count":2}`), &v); err != nil {
		t.Fatalf("client_version alone: %v", err)
	}
	if v.Title != "T" || v.Count != 2 {
		t.Fatalf("decoded %+v", v)
	}
	e := decodeErr(t, ctx, `{"client_version":"v1.9","working_dir":"/w"}`)
	want := bareUnknown + ": the caller is version v1.9 and the service is v1.8; run `code-central service restart`"
	if e.Message != want {
		t.Fatalf("got %q, want %q", e.Message, want)
	}
}

func TestClientVersionIsStrippedWithNoVersionsInTheContext(t *testing.T) {
	var v decodeTarget
	if err := core.Decode(context.Background(), []byte(`{"client_version":"v1.9","title":"T"}`), &v); err != nil || v.Title != "T" {
		t.Fatalf("decoded %+v: %v", v, err)
	}
}

func TestAClientVersionThatIsNotAStringIsAnUnknownField(t *testing.T) {
	e := decodeErr(t, context.Background(), `{"client_version":9}`)
	if e.Field != "client_version" {
		t.Fatalf("got %q field %q", e.Message, e.Field)
	}
}

func TestOnlyUnknownFieldsGetTheVersionHint(t *testing.T) {
	ctx := core.WithVersions(context.Background(), "v1.8", "v1.9")
	for in, want := range map[string]string{
		`{"count":"two"}`: "invalid JSON: json: cannot unmarshal string into Go struct field decodeTarget.count of type int",
		`{} {}`:           "invalid JSON: trailing data",
	} {
		if e := decodeErr(t, ctx, in); e.Message != want {
			t.Errorf("%s: got %q, want %q", in, e.Message, want)
		}
	}
}

func TestAnEmptyInputDecodesAsAnEmptyObject(t *testing.T) {
	var v decodeTarget
	if err := core.Decode(context.Background(), []byte("  "), &v); err != nil {
		t.Fatal(err)
	}
}
