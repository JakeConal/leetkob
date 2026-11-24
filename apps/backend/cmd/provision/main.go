// Command provision creates the five synthetic PA#3 assessment accounts,
// two classes and their memberships. It is idempotent: a repeated run never
// rewrites a password hash and never recreates removed memberships. It never
// creates a lab, submission or finding.
//
// Passwords are supplied per run through ASSESSMENT_PASSWORDS as JSON, so no
// credential is ever committed or baked into an image:
//
//	ASSESSMENT_PASSWORDS='{"admin":"...","taA":"...","taB":"...","studentA1":"...","studentA2":"..."}' \
//	DATABASE_URL=... go run ./cmd/provision
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/JakeConal/leetkob/backend/internal/access"
	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	rawPasswords := os.Getenv("ASSESSMENT_PASSWORDS")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "provision: DATABASE_URL is required")
		os.Exit(2)
	}
	if rawPasswords == "" {
		fmt.Fprintln(os.Stderr, "provision: ASSESSMENT_PASSWORDS is required (JSON, not committed)")
		os.Exit(2)
	}
	var passwords access.Passwords
	if err := json.Unmarshal([]byte(rawPasswords), &passwords); err != nil {
		fmt.Fprintf(os.Stderr, "provision: ASSESSMENT_PASSWORDS is not valid JSON: %v\n", err)
		os.Exit(2)
	}
	plan, err := access.BuildPlan(passwords)
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
		os.Exit(2)
	}

	ctx := context.Background()
	store, err := postgres.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()
	if err := store.Initialize(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
		os.Exit(1)
	}

	rows := access.AssessmentRows{
		Classes:     plan.Classes,
		Memberships: plan.Memberships,
	}
	for _, account := range plan.Accounts {
		hash, err := identity.HashPassword(account.Password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "provision: hash %s: %v\n", account.Email, err)
			os.Exit(1)
		}
		rows.Users = append(rows.Users, access.UserInsert{
			ID: account.ID, Email: account.Email, DisplayName: account.DisplayName,
			Role: account.Role, PasswordHash: hash,
		})
	}
	if err := store.ApplyAssessment(ctx, rows); err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Provisioned synthetic assessment directory:")
	for _, account := range plan.Accounts {
		class := "(no class)"
		for _, membership := range plan.Memberships {
			if membership.UserID == account.ID {
				for _, item := range plan.Classes {
					if item.ID == membership.ClassID {
						class = item.Name
					}
				}
			}
		}
		fmt.Printf("  %-34s %-8s %s\n", account.Email, account.Role, class)
	}
	fmt.Println("Passwords were read from ASSESSMENT_PASSWORDS and only hashes were stored.")
}
