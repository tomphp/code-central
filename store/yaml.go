package store

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// literalKeys are Markdown fields, written as literal block scalars.
var literalKeys = map[string]bool{
	"description": true, "body": true,
	"recommended": true, "answer": true,
	"what_broke": true, "why": true, "what_to_fix": true, "where_work_stands": true,
}

// timeKeys hold RFC 3339 timestamps, written unquoted.
var timeKeys = map[string]bool{"created": true, "updated": true, "time": true, "started": true, "ended": true, "posted": true, "closed": true, "archived": true}

// encodeYAML writes v (a struct) in its field order with two-space
// indentation, Markdown fields as literal block scalars.
func encodeYAML(v any) ([]byte, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	setLiteral(&n)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func setLiteral(n *yaml.Node) {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if literalKeys[k.Value] && v.Kind == yaml.ScalarNode && v.Value != "" {
				v.Style = yaml.LiteralStyle
			}
			if timeKeys[k.Value] && v.Kind == yaml.ScalarNode {
				v.Tag, v.Style = "!!timestamp", 0
			}
		}
	}
	for _, c := range n.Content {
		setLiteral(c)
	}
}

// decodeStrict decodes a YAML mapping into the struct out points to,
// refusing unknown and duplicate keys. On failure it returns the name of
// the offending field (nested fields as a[0].b). The store's own files
// take parseBlockYAML's fast path; anything else is parsed by yaml.v3.
func decodeStrict(data []byte, out any) (string, error) {
	doc, ok := parseBlockYAML(data)
	if !ok {
		doc = new(yaml.Node)
		if err := yaml.Unmarshal(data, doc); err != nil {
			return "file", fmt.Errorf("not valid YAML: %v", err)
		}
	}
	if len(doc.Content) == 0 {
		return "file", errors.New("empty")
	}
	field, err := decodeNode(doc.Content[0], reflect.ValueOf(out).Elem())
	return orFile(field), err
}

// decodeNode decodes n into v. On failure it returns the offending
// field's path relative to v, "" for v itself.
func decodeNode(n *yaml.Node, v reflect.Value) (string, error) {
	switch {
	case v.Kind() == reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return "", errors.New("must be a mapping")
		}
		fields := fieldsOf(v.Type())
		seen := make([]bool, len(fields))
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			idx, ok := fields[key]
			if !ok {
				return key, errors.New("unknown field")
			}
			if seen[idx] {
				return key, errors.New("duplicate field")
			}
			seen[idx] = true
			if f, err := decodeNode(n.Content[i+1], v.Field(idx)); err != nil {
				return key + subPath(f), err
			}
		}
		return "", nil
	case v.Kind() == reflect.Pointer && v.Type().Elem().Kind() == reflect.Struct:
		v.Set(reflect.New(v.Type().Elem()))
		return decodeNode(n, v.Elem())
	case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Struct:
		if n.Kind != yaml.SequenceNode {
			return "", errors.New("must be a list")
		}
		s := reflect.MakeSlice(v.Type(), len(n.Content), len(n.Content))
		for i, c := range n.Content {
			if f, err := decodeNode(c, s.Index(i)); err != nil {
				return fmt.Sprintf("[%d]", i) + subPath(f), err
			}
		}
		v.Set(s)
		return "", nil
	default:
		if decodeLeaf(n, v) {
			return "", nil
		}
		if err := n.Decode(v.Addr().Interface()); err != nil {
			return "", fmt.Errorf("wrong type: %v", err)
		}
		return "", nil
	}
}

// subPath joins a child's relative path onto its parent's.
func subPath(path string) string {
	if path == "" || path[0] == '[' {
		return path
	}
	return "." + path
}

// fieldIndexes caches each struct type's YAML keys and field indexes.
var fieldIndexes sync.Map // reflect.Type → map[string]int

// fieldsOf maps a struct type's YAML keys to its field indexes.
func fieldsOf(t reflect.Type) map[string]int {
	if f, ok := fieldIndexes.Load(t); ok {
		return f.(map[string]int)
	}
	fields := map[string]int{}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		fields[name] = i
	}
	fieldIndexes.Store(t, fields)
	return fields
}

// decodeLeaf decodes the common leaves of the store's files exactly as
// n.Decode would, without its per-call setup, and is false for anything
// else: a string or int scalar, and a list of them or a map of strings.
func decodeLeaf(n *yaml.Node, v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		s, ok := leafString(n)
		if ok {
			v.SetString(s)
		}
		return ok
	case reflect.Int:
		i, ok := leafInt(n)
		if ok {
			v.SetInt(int64(i))
		}
		return ok
	case reflect.Slice:
		if n.Kind != yaml.SequenceNode || (v.Type().Elem().Kind() != reflect.String && v.Type().Elem().Kind() != reflect.Int) {
			return false
		}
		s := reflect.MakeSlice(v.Type(), len(n.Content), len(n.Content))
		for i, c := range n.Content {
			if isNull(c) || !decodeLeaf(c, s.Index(i)) {
				return false // yaml.v3 leaves a null item out
			}
		}
		v.Set(s)
		return true
	case reflect.Map:
		if n.Kind != yaml.MappingNode || v.Type().Key().Kind() != reflect.String || v.Type().Elem().Kind() != reflect.String {
			return false
		}
		m := reflect.MakeMapWithSize(v.Type(), len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k, ok := leafString(n.Content[i])
			if !ok || k == "" || m.MapIndex(reflect.ValueOf(k)).IsValid() {
				return false // yaml.v3 names a duplicate key
			}
			val, ok := leafString(n.Content[i+1])
			if !ok {
				return false
			}
			m.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(val))
		}
		v.Set(m)
		return true
	}
	return false
}

// leafString is a scalar read into a string: its text, or "" for null.
func leafString(n *yaml.Node) (string, bool) {
	if n.Kind != yaml.ScalarNode {
		return "", false
	}
	switch n.Style {
	case yaml.DoubleQuotedStyle, yaml.SingleQuotedStyle, yaml.LiteralStyle, yaml.FoldedStyle:
		return n.Value, n.Tag == "" || n.Tag == "!!str"
	case 0:
		if isNull(n) {
			return "", true
		}
		return n.Value, n.Value != "<<" && n.Tag != "!!null"
	}
	return "", false
}

// isNull is whether n is a plain null scalar.
func isNull(n *yaml.Node) bool {
	if n.Kind != yaml.ScalarNode || n.Style != 0 || (n.Tag != "" && n.Tag != "!!null") {
		return false
	}
	switch n.Value {
	case "", "~", "null", "Null", "NULL":
		return true
	}
	return false
}

// leafInt is a plain decimal scalar read into an int. Past 18 digits
// it may overflow, which yaml.v3 reports, so those fall back.
func leafInt(n *yaml.Node) (int, bool) {
	if n.Kind != yaml.ScalarNode || n.Style != 0 || (n.Tag != "" && n.Tag != "!!int") {
		return 0, false
	}
	s := strings.TrimPrefix(n.Value, "-")
	if s == "" || len(s) > 18 || (s[0] == '0' && len(s) > 1) {
		return 0, false
	}
	for _, c := range []byte(s) {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	i, err := strconv.Atoi(n.Value)
	return i, err == nil
}

func orFile(path string) string {
	if path == "" {
		return "file"
	}
	return path
}
