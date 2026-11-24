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
	"errors"
	"fmt"
	"os"

	"github.com/JakeConal/leetkob/backend/internal/access"
	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

func main() {
	databaseURL, passwords, err := assessmentConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
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
	if err := applyPlan(ctx, store, plan); err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
		os.Exit(1)
	}
	printPlan(plan)
}

// assessmentConfig reads the database URL and the operator-supplied
// passwords. Passwords never come from a file in the repository.
func assessmentConfig() (string, access.Passwords, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return "", access.Passwords{}, errors.New("DATABASE_URL is required")
	}
	rawPasswords := os.Getenv("ASSESSMENT_PASSWORDS")
	if rawPasswords == "" {
		return "", access.Passwords{}, errors.New("ASSESSMENT_PASSWORDS is required (JSON, not committed)")
	}
	var passwords access.Passwords
	if err := json.Unmarshal([]byte(rawPasswords), &passwords); err != nil {
		return "", access.Passwords{}, fmt.Errorf("ASSESSMENT_PASSWORDS is not valid JSON: %w", err)
	}
	return databaseURL, passwords, nil
}

// applyPlan hashes the passwords and writes the synthetic directory in one
// idempotent call.
func applyPlan(ctx context.Context, store *postgres.Store, plan access.Plan) error {
	rows := access.AssessmentRows{
		Classes:     plan.Classes,
		Memberships: plan.Memberships,
	}
	for _, account := range plan.Accounts {
		hash, err := identity.HashPassword(account.Password)
		if err != nil {
			return fmt.Errorf("hash %s: %w", account.Email, err)
		}
		rows.Users = append(rows.Users, access.UserInsert{
			ID: account.ID, Email: account.Email, DisplayName: account.DisplayName,
			Role: account.Role, PasswordHash: hash,
		})
	}
	return store.ApplyAssessment(ctx, rows)
}

func printPlan(plan access.Plan) {
	fmt.Println("Provisioned synthetic assessment directory:")
	for _, account := range plan.Accounts {
		fmt.Printf("  %-34s %-8s %s\n", account.Email, account.Role, classOf(plan, account.ID))
	}
	fmt.Println("Passwords were read from ASSESSMENT_PASSWORDS and only hashes were stored.")
}

func classOf(plan access.Plan, userID string) string {
	for _, membership := range plan.Memberships {
		if membership.UserID != userID {
			continue
		}
		for _, item := range plan.Classes {
			if item.ID == membership.ClassID {
				return item.Name
			}
		}
	}
	return "(no class)"
}
