//go:build !js

// A database Store as a File Store layout, and back (Spec
// code-central-1032). The exporter writes each row's document as the file
// it stands for, at whatever format the database is at, without decoding
// it, so a Migration can stage an old database as files and run the one
// chain of steps over them. The importer loads a File Store, at this
// build's format, into an empty database in one transaction. Usage goes
// both ways line by line, each the same JSON, a line no read could parse
// included.
//
// The exporter reads the tables as this build lays them out. A format
// that changes the tables, not only the documents, must have it read the
// older layout too, or no older database could be migrated.

package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// exportFiles writes every record and Usage record q's database holds into
// dir as a File Store layout at format, the database's own.
func exportFiles(ctx context.Context, q querier, d dialect, dir string, format int) error {
	x := exporter{ctx: ctx, q: q, d: d, dir: dir}
	if err := x.write("store.yaml", fmt.Appendf(nil, "format: %d\n", format)); err != nil {
		return err
	}
	x.rows(`SELECT doc FROM options WHERE id = 1`, func(scan func(...any) error) error {
		var doc []byte
		if err := scan(&doc); err != nil {
			return err
		}
		return x.yamlFile("options.yaml", doc, nil)
	})
	x.rows(`SELECT key, doc FROM projects`, func(scan func(...any) error) error {
		var key string
		var doc []byte
		if err := scan(&key, &doc); err != nil {
			return err
		}
		return x.yamlFile(filepath.Join("projects", key, "project.yaml"), doc, nil)
	})
	x.rows(`SELECT project, doc FROM ci_runs`, func(scan func(...any) error) error {
		var project string
		var doc []byte
		if err := scan(&project, &doc); err != nil {
			return err
		}
		return x.yamlFile(filepath.Join("projects", project, ciRunsName), doc, nil)
	})
	x.rows(`SELECT project, number, doc FROM specs`, func(scan func(...any) error) error {
		var project string
		var n int
		var doc []byte
		if err := scan(&project, &n, &doc); err != nil {
			return err
		}
		return x.specFile(filepath.Join("projects", project, "specs", strconv.Itoa(n)+".md"), doc)
	})
	cols := make([]string, len(History))
	for i, p := range History {
		cols[i] = string(p)
	}
	x.rows(`SELECT project, number, doc, `+strings.Join(cols, ", ")+` FROM tickets`, func(scan func(...any) error) error {
		var project string
		var n int
		var doc []byte
		parts := make([][]byte, len(History))
		dest := []any{&project, &n, &doc}
		for i := range parts {
			dest = append(dest, &parts[i])
		}
		if err := scan(dest...); err != nil {
			return err
		}
		return x.yamlFile(filepath.Join("projects", project, "tickets", strconv.Itoa(n)+".yaml"), doc, func(node *yaml.Node) error {
			for i, p := range History {
				pn, err := jsonNode(parts[i])
				if err != nil {
					return fmt.Errorf("%s: %w", p, err)
				}
				if !isNull(pn) { // a part the file left out
					putKey(node, string(p), pn)
				}
			}
			return nil
		})
	})
	usageFiles := map[string]*bytes.Buffer{}
	x.rows(`SELECT project, day, type, doc FROM usage_records ORDER BY id`, func(scan func(...any) error) error {
		var project, day, typ string
		var doc []byte
		if err := scan(&project, &day, &typ, &doc); err != nil {
			return err
		}
		rel := filepath.Join("projects", project, "usage", day+".jsonl")
		if usageFiles[rel] == nil {
			usageFiles[rel] = &bytes.Buffer{}
		}
		usageFiles[rel].Write(usageLine(typ, doc))
		usageFiles[rel].WriteByte('\n')
		return nil
	})
	for rel, buf := range usageFiles {
		if x.err == nil {
			x.err = x.write(rel, buf.Bytes())
		}
	}
	return x.err
}

// exporter writes the files of one export, keeping its first failure.
type exporter struct {
	ctx context.Context
	q   querier
	d   dialect
	dir string
	err error
}

// rows runs query and fn over each row it returns, unless an earlier one
// failed.
func (x *exporter) rows(query string, fn func(scan func(...any) error) error) {
	if x.err != nil {
		return
	}
	rows, err := x.q.QueryContext(x.ctx, x.d.rebind(query))
	if err != nil {
		x.err = err
		return
	}
	defer rows.Close()
	for rows.Next() {
		if x.err = fn(rows.Scan); x.err != nil {
			return
		}
	}
	x.err = rows.Err()
}

// yamlFile writes the JSON document doc as the YAML file rel, after edit,
// if any, has added to its node tree.
func (x *exporter) yamlFile(rel string, doc []byte, edit func(*yaml.Node) error) error {
	data, err := func() ([]byte, error) {
		node, err := jsonNode(doc)
		if err == nil && edit != nil {
			err = edit(node)
		}
		if err != nil {
			return nil, err
		}
		return yamlOfDoc(node)
	}()
	return x.file(rel, data, err)
}

// specFile writes the Spec document doc as its Markdown file: its
// frontmatter, then its body.
func (x *exporter) specFile(rel string, doc []byte) error {
	data, err := func() ([]byte, error) {
		node, err := jsonNode(doc)
		if err != nil {
			return nil, err
		}
		body := takeKey(node, "body")
		if body == nil {
			return nil, errors.New("the document has no body")
		}
		front, err := yamlOfDoc(node)
		return append(append(append([]byte("---\n"), front...), "---\n"...), body.Value...), err
	}()
	return x.file(rel, data, err)
}

// file writes data as the file rel, or names rel in err, the failure to
// make data.
func (x *exporter) file(rel string, data []byte, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.ToSlash(rel), err)
	}
	return x.write(rel, data)
}

// write writes data as the file rel of the layout, making its folder.
func (x *exporter) write(rel string, data []byte) error {
	path := filepath.Join(x.dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// yamlOfDoc is the node tree of a JSON document as its file's YAML, as
// encodeYAML writes a record: strings plain unless they need quotes,
// Markdown fields literal and times unquoted.
func yamlOfDoc(n *yaml.Node) ([]byte, error) {
	plainStrings(n)
	setLiteral(n)
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(n); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// plainStrings lets the encoder choose each string's style, which quotes
// one only where it would read as another type.
func plainStrings(n *yaml.Node) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!str" {
		n.Style = 0
	}
	for _, c := range n.Content {
		plainStrings(c)
	}
}

// usageLine is a Usage row as its line in a File Store's usage file: the
// document with its type first. A row without a type is a line no read
// could parse, kept as it was.
func usageLine(typ string, doc []byte) []byte {
	if typ == "" {
		return doc
	}
	t, _ := json.Marshal(typ)
	line := append([]byte(`{"type":`), t...)
	rest := bytes.TrimSpace(doc)
	rest = bytes.TrimPrefix(rest, []byte("{"))
	if r := bytes.TrimSpace(rest); !bytes.Equal(r, []byte("}")) {
		line = append(line, ',')
	}
	return append(line, rest...)
}

// usageRow is a line of a File Store's usage file as its row's type and
// document, which usageLine gives back.
func usageRow(line []byte) (typ string, doc []byte) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(line, &fields) != nil || json.Unmarshal(fields["type"], &typ) != nil || typ == "" {
		return "", line
	}
	delete(fields, "type")
	doc, err := json.Marshal(fields)
	if err != nil {
		return "", line
	}
	return typ, doc
}

// importFiles writes every record of from, a File Store at this build's
// format, and its Usage, into the empty database behind q, which is in a
// transaction: the import lands whole or not at all.
func importFiles(ctx context.Context, q querier, d dialect, from *FileStore) error {
	write := func(fn func(w *sqlWriter) error) error {
		var failed error
		err := fn(&sqlWriter{sqlReader: sqlReader{q: q, d: d, err: &failed}})
		if err == nil {
			err = failed
		}
		return err
	}
	err := from.View(func(r Reader) error {
		if err := write(func(w *sqlWriter) error { return w.PutOptions(r.Options()) }); err != nil {
			return err
		}
		for _, p := range r.Projects() {
			if err := write(func(w *sqlWriter) error { return w.PutProject(p) }); err != nil {
				return err
			}
			for rec := range r.Records(p.Key) {
				err := write(func(w *sqlWriter) error {
					if rec.Spec != nil {
						return w.writeSpec(p.Key, *rec.Spec)
					}
					return w.writeTicket(p.Key, withHead(*rec.Ticket))
				})
				if err != nil {
					return err
				}
			}
			if runs := r.CIRuns(p.Key); len(runs) > 0 {
				if err := write(func(w *sqlWriter) error { return w.PutCIRuns(p.Key, runs) }); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return importUsage(ctx, q, d, from.Dir())
}

// importUsage writes each line of the File Store's usage files at dir as
// a Usage row, in the order of the lines.
func importUsage(ctx context.Context, q querier, d dialect, dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "projects", "*", "usage", "*.jsonl"))
	if err != nil {
		return err
	}
	for _, f := range files {
		project := filepath.Base(filepath.Dir(filepath.Dir(f)))
		day := strings.TrimSuffix(filepath.Base(f), ".jsonl")
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		for line := range bytes.SplitSeq(data, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			typ, doc := usageRow(line)
			if _, err := q.ExecContext(ctx, d.rebind(`INSERT INTO usage_records (project, day, type, doc) VALUES (?, ?, ?, ?)`), project, day, typ, string(doc)); err != nil {
				return err
			}
		}
	}
	return nil
}
