// Command provision creates the five synthetic PA#3 assessment accounts,
// their two classes and memberships. It is idempotent: existing accounts
// keep their credentials and a repeated run never silently resets a
// password. It never creates the marked lab or any submission.
//
// Usage: DATABASE_URL=... go run ./cmd/provision
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/JakeConal/leetkob/backend/internal/adapters/postgres"
	"github.com/JakeConal/leetkob/backend/internal/auth"
	"github.com/JakeConal/leetkob/backend/internal/identity"
)

const (
	classAID   = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	classBID   = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	classNameA = "PA3 Class A"
	classNameB = "PA3 Class B"
)

func seeds() []postgres.ProvisionAccountSeed {
	return []postgres.ProvisionAccountSeed{
		{Email: "admin@leetkob.local", DisplayName: "PA3 Admin", Role: auth.RoleAdmin},
		{Email: "ta1@leetkob.local", DisplayName: "PA3 TA One", Role: auth.RoleTA, ClassID: classAID, ClassName: classNameA},
		{Email: "ta2@leetkob.local", DisplayName: "PA3 TA Two", Role: auth.RoleTA, ClassID: classBID, ClassName: classNameB},
		{Email: "student1@leetkob.local", DisplayName: "PA3 Student One", Role: auth.RoleStudent, ClassID: classAID, ClassName: classNameA},
		{Email: "student2@leetkob.local", DisplayName: "PA3 Student Two", Role: auth.RoleStudent, ClassID: classAID, ClassName: classNameA},
	}
}

func main() {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "provision: DATABASE_URL is required")
		os.Exit(2)
	}
	ctx := context.Background()
	store, err := postgres.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()
	if err := postgres.Migrate(ctx, store.Pool()); err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
		os.Exit(1)
	}

	results, err := store.ProvisionAccounts(ctx, seeds(), identity.HashPassword)
	if err != nil {
		fmt.Fprintf(os.Stderr, "provision: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("PROVISIONED ACCOUNTS (synthetic assessment data only)")
	fmt.Println("email\trole\tclass\tpassword")
	for _, account := range results {
		class := account.ClassName
		if class == "" {
			class = "-"
		}
		password := "(unchanged)"
		if account.Created {
			password = account.Password
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", account.Email, account.Role, class, password)
	}
	fmt.Println("Record newly created passwords now: they are not stored in plaintext and are printed once.")
}
