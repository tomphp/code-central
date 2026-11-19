// The records as JSON documents (Spec code-central-1032): the database
// Stores keep each record as JSON of the same keys and values its YAML
// file holds, so the two Store Kinds share one versioned record shape
// and one chain of Migrations. A document is encoded from the record's
// file struct through its yaml tags, and decoded through the same strict
// decoder the files use, so an unknown key fails in both.

package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"

	"gopkg.in/yaml.v3"
)

// jsonDoc is v, a file struct, as a JSON document: its yaml keys in field
// order, lists and maps as JSON arrays and objects, and nothing it would
// leave out of the file.
func jsonDoc(v any) ([]byte, error) {
	n, err := docNode(v)
	if err != nil {
		return nil, err
	}
	return jsonOf(n)
}

// docNode is v, a file struct, as the node tree its file would hold.
func docNode(v any) (*yaml.Node, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	return &n, nil
}

// jsonOf is the node tree n as a JSON document.
func jsonOf(n *yaml.Node) ([]byte, error) {
	var buf bytes.Buffer
	if err := writeJSON(&buf, n); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// takeKey removes key from the mapping n and returns its value, or nil
// when n has no such key.
func takeKey(n *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			v := n.Content[i+1]
			n.Content = append(n.Content[:i], n.Content[i+2:]...)
			return v
		}
	}
	return nil
}

// putKey adds key with the value v to the mapping n.
func putKey(n *yaml.Node, key string, v *yaml.Node) {
	n.Content = append(n.Content, stringNode(key), v)
}

// stringNode is s as a scalar node.
func stringNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Style: yaml.DoubleQuotedStyle, Value: s}
}

// writeJSON writes the node n as JSON.
func writeJSON(buf *bytes.Buffer, n *yaml.Node) error {
	switch n.Kind {
	case yaml.DocumentNode:
		return writeJSON(buf, n.Content[0])
	case yaml.MappingNode:
		buf.WriteByte('{')
		for i := 0; i+1 < len(n.Content); i += 2 {
			if i > 0 {
				buf.WriteByte(',')
			}
			key, err := json.Marshal(n.Content[i].Value)
			if err != nil {
				return err
			}
			buf.Write(key)
			buf.WriteByte(':')
			if err := writeJSON(buf, n.Content[i+1]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case yaml.SequenceNode:
		buf.WriteByte('[')
		for i, c := range n.Content {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeJSON(buf, c); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!int", "!!float", "!!bool":
			buf.WriteString(n.Value)
		case "!!null":
			buf.WriteString("null")
		default:
			s, err := json.Marshal(n.Value)
			if err != nil {
				return err
			}
			buf.Write(s)
		}
	default:
		return fmt.Errorf("store: cannot write a %v node as JSON", n.Kind)
	}
	return nil
}

// fromJSONDoc decodes the JSON document data into the file struct out
// points to, strictly, as decodeStrict decodes a file: on failure it
// names the offending field.
func fromJSONDoc(data []byte, out any) (string, error) {
	n, err := jsonNode(data)
	if err != nil {
		return "file", fmt.Errorf("not valid JSON: %v", err)
	}
	return decodeDocNode(n, out)
}

// jsonNode parses the JSON document data into the node tree the strict
// decoder reads, keeping the keys' order.
func jsonNode(data []byte) (*yaml.Node, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	n, err := readJSONValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing data")
	}
	return n, nil
}

// readJSONValue reads one JSON value from dec as the node the strict
// decoder reads: an object as a mapping in key order, an array as a
// sequence, and each scalar with the tag its JSON type gives it, so a
// string is never taken for a number or a bool.
func readJSONValue(dec *json.Decoder) (*yaml.Node, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch v := tok.(type) {
	case json.Delim:
		switch v {
		case '{':
			n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					return nil, err
				}
				k, ok := key.(string)
				if !ok {
					return nil, fmt.Errorf("object key %v is not a string", key)
				}
				val, err := readJSONValue(dec)
				if err != nil {
					return nil, err
				}
				n.Content = append(n.Content, stringNode(k), val)
			}
			if _, err := dec.Token(); err != nil { // the closing brace
				return nil, err
			}
			return n, nil
		case '[':
			n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
			for dec.More() {
				val, err := readJSONValue(dec)
				if err != nil {
					return nil, err
				}
				n.Content = append(n.Content, val)
			}
			if _, err := dec.Token(); err != nil { // the closing bracket
				return nil, err
			}
			return n, nil
		}
		return nil, fmt.Errorf("unexpected %v", v)
	case string:
		return stringNode(v), nil
	case json.Number:
		tag := "!!int"
		if _, err := strconv.ParseInt(v.String(), 10, 64); err != nil {
			tag = "!!float"
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: v.String()}, nil
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v)}, nil
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}, nil
	}
	return nil, fmt.Errorf("unexpected token %v", tok)
}

// decodeDocNode decodes the node tree n, as jsonNode read it, into the
// file struct out points to, strictly: on failure it names the offending
// field.
func decodeDocNode(n *yaml.Node, out any) (string, error) {
	field, err := decodeNode(n, reflect.ValueOf(out).Elem())
	return orFile(field), err
}
