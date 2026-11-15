package main

import (
	"reflect"

	"github.com/tomoram/code-central/core"
	"github.com/tomoram/code-central/store"
)

// roots are the response types the UI reads, each written with every
// type it reaches. An operation that returns a new top-level view adds
// its type here.
var roots = []reflect.Type{
	reflect.TypeFor[core.Acknowledgement](),
	reflect.TypeFor[core.TargetView](),
	reflect.TypeFor[core.UsageView](),
}

// enums are the value lists of the typed strings the roots reach, each
// written as a union with its value array.
var enums = []Enum{
	enumOf("States", store.States),
	enumOf("AutoAcceptKinds", store.AutoAcceptKinds),
}
