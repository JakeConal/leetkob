package hardrule

import "github.com/JakeConal/leetkob/backend/internal/checker/schema"

// Result is analyzer-result.v1. Status unsure is unsupported analysis.
type Result struct {
	Version    int    `json:"version"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
	References []Ref  `json:"references"`
}

// Ref is one resolved use, include, or unresolved identifier in the
// submitted file. Line is a one-based location in that file.
type Ref struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualifiedName"`
	Header        string `json:"header"`
	Resolved      bool   `json:"resolved"`
	UserDeclared  bool   `json:"userDeclared"`
	Line          int    `json:"line"`
	Column        int    `json:"column"`
}

// Decision is the hard-rule outcome for one constraint. HardRule is false
// when the constraint is not a ban list; the caller may then use a semantic
// check. A ban list that cannot be decided has Verdict unsure and must not
// be reported as a violation.
type Decision struct {
	HardRule bool
	Verdict  schema.Verdict
	Evidence []schema.Evidence
	Reason   string
}

// MaxReferences is the analyzer-result.v1 bound. A larger document is unsure.
const MaxReferences = 4096

// MaxSourceBytes is the submission source bound. Larger input is unsure.
const MaxSourceBytes = 65536
