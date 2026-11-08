package cli

import (
	"context"

	"github.com/tomoram/code-central/core"
)

// Scratch Import (ticket 40). `import-guide` is the spec's name for the
// guide command; `get-import-guide` is the operation's kebab-case name.
func init() {
	commands["get-import-guide"] = command{op: "get_import_guide", usage: "get-import-guide  print how to map a .scratch/ folder (same as import-guide)", run: runImportGuide}
	commands["import-guide"] = command{usage: "import-guide  print how to map a .scratch/ folder into an import", run: runImportGuide}
	commands["import"] = command{op: "import", usage: "import [--dry-run] [--file <json-path|->]  import a .scratch/ payload {specs, tickets} from stdin or a file", run: runImport}
}

func runImportGuide(ctx context.Context, e Env, args []string) error {
	fs := flagSet("import-guide")
	project := addScope(fs)
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	sc, err := resolveScope(e, *project)
	if err != nil {
		return err
	}
	v, err := sc.client.GetImportGuide(ctx, sc.project)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}

// runImport reads the payload (the JSON object {specs, tickets}) from
// --file, or from stdin when --file is absent or -. It is decoded as
// strictly as the service would, so a fault comes back as invalid.
func runImport(ctx context.Context, e Env, args []string) error {
	fs := flagSet("import")
	project := addScope(fs)
	dryRun := fs.Bool("dry-run", false, "")
	file := fs.String("file", "-", "")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		return usagef("unexpected argument %q", pos[0])
	}
	sc, err := resolveScope(e, *project)
	if err != nil {
		return err
	}
	text, err := longInput(e, "", *file)
	if err != nil {
		return err
	}
	in := core.ImportInput{DryRun: *dryRun}
	if err := core.Decode(ctx, []byte(text), &in.Payload); err != nil {
		return err
	}
	v, err := sc.client.Import(ctx, sc.project, in)
	if err != nil {
		return err
	}
	return printJSON(e, v)
}
