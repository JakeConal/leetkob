package access

import (
	"context"
	"fmt"

	"github.com/JakeConal/leetkob/backend/internal/identity"
)

// Provision writes the synthetic assessment directory. A second call does
// not add rows, does not replace a stored password hash, and does not
// insert a lab or a submission. A membership that was removed stays
// removed; the insert is skipped when any row for that pair already exists.
func Provision(ctx context.Context, repo Repository, passwords Passwords) error {
	plan, err := BuildPlan(passwords)
	if err != nil {
		return err
	}
	rows := AssessmentRows{
		Classes:     plan.Classes,
		Memberships: plan.Memberships,
		Users:       make([]UserInsert, 0, len(plan.Accounts)),
	}
	for _, account := range plan.Accounts {
		hash, err := identity.HashPassword(account.Password)
		if err != nil {
			return fmt.Errorf("access: hash password: %w", err)
		}
		rows.Users = append(rows.Users, UserInsert{
			ID:           account.ID,
			Email:        account.Email,
			DisplayName:  account.DisplayName,
			Role:         account.Role,
			PasswordHash: hash,
		})
	}
	if err := repo.ApplyAssessment(ctx, rows); err != nil {
		return fmt.Errorf("access: provision: %w", err)
	}
	return nil
}
