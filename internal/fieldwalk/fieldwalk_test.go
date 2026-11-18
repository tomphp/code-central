package fieldwalk

import (
	"reflect"
	"slices"
	"testing"
)

type base struct {
	ID string `json:"id"`
}

type row struct {
	base
	Count    int    `json:"count,omitempty"`
	Secret   string `json:"-"`
	hidden   string
	Untagged bool
	Zeroed   base `json:"zeroed,omitzero"`
}

func TestNamesFlattenEmbeddedStructsAndSkipHiddenFields(t *testing.T) {
	got := Names(reflect.TypeFor[row]())
	want := []string{"id", "count", "Untagged", "zeroed"}
	if !slices.Equal(got, want) {
		t.Fatalf("names %v, want %v", got, want)
	}
	if got := Names(reflect.TypeFor[*row]()); !slices.Equal(got, want) {
		t.Fatalf("names through a pointer %v, want %v", got, want)
	}
	if got := Names(reflect.TypeFor[string]()); got != nil {
		t.Fatalf("a string has names %v", got)
	}
}

func TestFieldsAddressEmbeddedFieldsByIndex(t *testing.T) {
	v := row{base: base{ID: "r-1"}, Count: 2}
	fields := Fields(reflect.TypeOf(v))
	if fields[0].Name != "id" || reflect.ValueOf(v).FieldByIndex(fields[0].Index).String() != "r-1" {
		t.Fatalf("id: %+v", fields[0])
	}
	if !fields[1].OmitEmpty || fields[1].Index[0] != 1 {
		t.Fatalf("count: %+v", fields[1])
	}
	if !fields[3].OmitEmpty {
		t.Fatalf("omitzero should count as OmitEmpty: %+v", fields[3])
	}
}

func TestOfReadsOneField(t *testing.T) {
	rt := reflect.TypeFor[row]()
	for _, c := range []struct {
		field string
		want  Info
	}{
		{"base", Info{Flattened: true}},
		{"Count", Info{Name: "count", Options: []string{"omitempty"}, OmitEmpty: true}},
		{"Secret", Info{Name: "-", Skipped: true}},
		{"hidden", Info{Skipped: true}},
		{"Untagged", Info{Name: "Untagged"}},
	} {
		f, _ := rt.FieldByName(c.field)
		if got := Of(f); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.field, got, c.want)
		}
	}
}
