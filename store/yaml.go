package store

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// literalKeys are Markdown fields, written as literal block scalars.
var literalKeys = map[string]bool{
	"description": true, "body": true,
	"recommended": true, "answer": true,
	"what_broke": true, "why": true, "what_to_fix": true, "where_work_stands": true,
}

// timeKeys hold RFC 3339 timestamps, written unquoted.
var timeKeys = map[string]bool{"created": true, "updated": true, "time": true, "started": true, "ended": true, "posted": true, "closed": true}

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
// the offending field (nested fields as a[0].b).
func decodeStrict(data []byte, out any) (string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "file", fmt.Errorf("not valid YAML: %v", err)
	}
	if len(doc.Content) == 0 {
		return "file", errors.New("empty")
	}
	return decodeNode(doc.Content[0], reflect.ValueOf(out).Elem(), "")
}

func decodeNode(n *yaml.Node, v reflect.Value, path string) (string, error) {
	switch {
	case v.Kind() == reflect.Struct:
		if n.Kind != yaml.MappingNode {
			return orFile(path), errors.New("must be a mapping")
		}
		fields := map[string]int{}
		for i := 0; i < v.NumField(); i++ {
			name, _, _ := strings.Cut(v.Type().Field(i).Tag.Get("yaml"), ",")
			fields[name] = i
		}
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			fieldPath := key
			if path != "" {
				fieldPath = path + "." + key
			}
			idx, ok := fields[key]
			if !ok {
				return fieldPath, errors.New("unknown field")
			}
			if seen[key] {
				return fieldPath, errors.New("duplicate field")
			}
			seen[key] = true
			if f, err := decodeNode(n.Content[i+1], v.Field(idx), fieldPath); err != nil {
				return f, err
			}
		}
		return "", nil
	case v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Struct:
		if n.Kind != yaml.SequenceNode {
			return orFile(path), errors.New("must be a list")
		}
		s := reflect.MakeSlice(v.Type(), len(n.Content), len(n.Content))
		for i, c := range n.Content {
			if f, err := decodeNode(c, s.Index(i), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return f, err
			}
		}
		v.Set(s)
		return "", nil
	default:
		if err := n.Decode(v.Addr().Interface()); err != nil {
			return orFile(path), fmt.Errorf("wrong type: %v", err)
		}
		return "", nil
	}
}

func orFile(path string) string {
	if path == "" {
		return "file"
	}
	return path
}
