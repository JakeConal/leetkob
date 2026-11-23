package db

import (
	"strings"
	"testing"
)

// Tables the later PA#3 flow needs. Initialization may leave them empty.
var requiredTables = []string{
	"schema_migrations",
	"users",
	"sessions",
	"classes",
	"memberships",
	"labs",
	"lab_revisions",
	"lab_tests",
	"lab_constraints",
	"submissions",
	"counted_selections",
	"judge_verdicts",
	"judge_result_tests",
	"jobs",
	"findings",
	"finding_evidence",
	"decisions",
	"replays",
	"budget_reservations",
	"admission_leases",
	"auth_rate_limits",
}

func TestInitSQLDeclaresRequiredTables(t *testing.T) {
	script := strings.ToLower(stripSQLComments(InitSQL))
	if strings.TrimSpace(script) == "" {
		t.Fatal("initialization script is empty")
	}
	for _, table := range requiredTables {
		needle := "create table if not exists " + table
		if !strings.Contains(script, needle) {
			t.Fatalf("script missing %q", needle)
		}
	}
	if !strings.Contains(script, "token_hash") {
		t.Fatal("sessions must store a token hash")
	}
	if strings.Contains(script, "create trigger") || strings.Contains(script, "execute function") || strings.Contains(script, "execute procedure") {
		t.Fatal("initialization must not install triggers")
	}
}

func TestInitSQLHasNoDestructiveStatements(t *testing.T) {
	script := stripSQLComments(InitSQL)
	for _, stmt := range strings.Split(script, ";") {
		fields := strings.Fields(strings.ToLower(stmt))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "drop", "truncate", "delete":
			t.Fatalf("destructive statement %q", fields[0])
		}
		if fields[0] == "alter" {
			for _, field := range fields[1:] {
				if field == "drop" {
					t.Fatal("destructive ALTER ... DROP")
				}
			}
		}
	}
}

func stripSQLComments(script string) string {
	var b strings.Builder
	for len(script) > 0 {
		switch {
		case strings.HasPrefix(script, "--"):
			if i := strings.IndexByte(script, '\n'); i >= 0 {
				script = script[i+1:]
			} else {
				return b.String()
			}
		case strings.HasPrefix(script, "/*"):
			if i := strings.Index(script, "*/"); i >= 0 {
				script = script[i+2:]
			} else {
				return b.String()
			}
		default:
			b.WriteByte(script[0])
			script = script[1:]
		}
	}
	return b.String()
}
