package ops

import (
	"github.com/tomoram/code-central/api"
)

// Scratch Import (ticket 40). Import writes as agent whatever Actor the
// request names; core sees to that. The payload's full schema is in the
// guide, and core validates it strictly, naming the field of any fault.

// GetImportGuide is get_import_guide.
var GetImportGuide = &Definition[struct{}, api.ImportGuideView]{
	Name: "get_import_guide", Audience: AgentAndHuman,
	HTTP: "GET /api/p/{key}/import-guide",
	Doc:  "Get how to map a repository's .scratch/ folder into one import payload, and how to present the plan to the human.",
}

// Import is import.
var Import = &Definition[api.ImportInput, api.ImportResult]{
	Name: "import", Audience: AgentAndHuman, Write: true,
	HTTP: "POST /api/p/{key}/import",
	Doc:  "Import a whole .scratch/ folder mapped as get_import_guide says; run with dry_run first and wait for the human's go-ahead.",
}
