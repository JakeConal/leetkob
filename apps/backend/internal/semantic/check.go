package semantic

import (
	"context"
	"errors"
)

// Check sanitizes outbound fields, reserves the conservative call price
// against the USD 0.25 lab cap, and dispatches at most twice. Schema
// repair is the only retry. Timeout, provider failure, unknown usage,
// and a reservation that does not fit never borrow a further call from
// a later attempt beyond that single repair.
func (c *Checker) Check(ctx context.Context, in Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if c == nil || c.Model == nil || c.Store == nil {
		return Result{}, errors.New("semantic: checker is missing model or store")
	}
	if c.Timeout <= 0 {
		return Result{}, errors.New("semantic: call deadline is required")
	}
	if in.LabID == "" || len(in.Constraints) == 0 {
		return Result{}, errors.New("semantic: lab and constraints are required")
	}
	sanitized, err := Sanitize(SanitizeInput{
		Statement:      in.Statement,
		Constraints:    in.Constraints,
		Source:         in.Source,
		RedactionTerms: in.RedactionTerms,
	})
	if err != nil {
		return Result{Findings: unsureFindings(in, CausePreparation)}, nil
	}
	maxCost, priced := in.Price.ConservativeMicroUSD()
	if !priced {
		return Result{Findings: unsureFindings(in, CausePricing)}, nil
	}

	var calls int
	var lastCause UnsureCause
	for attempt := 1; attempt <= 2; attempt++ {
		reservation, err := c.Store.Reserve(ctx, in.LabID, maxCost, LabCapMicroUSD)
		if errors.Is(err, ErrBudgetExceeded) {
			if attempt == 1 {
				return Result{Findings: unsureFindings(in, CauseBudget)}, nil
			}
			return Result{Findings: unsureFindings(in, lastCause), ModelCalls: calls}, nil
		}
		if err != nil {
			return Result{}, err
		}
		system := sanitized.System
		if attempt == 2 {
			system += RepairNote
		}
		callCtx, cancel := context.WithTimeout(ctx, c.Timeout)
		resp, callErr := c.Model.Complete(callCtx, CompletionRequest{
			System:          system,
			UserJSON:        sanitized.UserJSON,
			Repair:          attempt == 2,
			MaxOutputTokens: in.Price.MaxOutputTokens,
		})
		cancel()
		calls++
		if callErr != nil {
			if err := c.Store.Keep(ctx, reservation.ID); err != nil {
				return Result{}, err
			}
			if errors.Is(callErr, context.DeadlineExceeded) {
				return Result{Findings: unsureFindings(in, CauseTimeout), ModelCalls: calls}, nil
			}
			if errors.Is(callErr, context.Canceled) {
				return Result{}, callErr
			}
			return Result{Findings: unsureFindings(in, CauseProvider), ModelCalls: calls}, nil
		}
		findings, parseErr := parseModelOutput(resp.Content, in.Constraints, sanitized.OriginalLines, sanitized.SanitizedLines)
		if err := c.account(ctx, reservation, in.Price, resp); err != nil {
			return Result{}, err
		}
		if parseErr == nil {
			return Result{Findings: findings, ModelCalls: calls}, nil
		}
		if errors.Is(parseErr, ErrEvidenceMismatch) {
			lastCause = CauseEvidence
		} else {
			lastCause = CauseSchema
		}
		if attempt == 2 {
			return Result{Findings: unsureFindings(in, lastCause), ModelCalls: calls}, nil
		}
	}
	return Result{Findings: unsureFindings(in, lastCause), ModelCalls: calls}, nil
}

func (c *Checker) account(ctx context.Context, reservation Reservation, price Price, resp ModelResponse) error {
	actual, known := usageMicroUSD(price, resp)
	if !known || actual > reservation.AmountMicroUSD {
		return c.Store.Keep(ctx, reservation.ID)
	}
	return c.Store.Settle(ctx, reservation.ID, actual)
}

func unsureFindings(in Input, cause UnsureCause) []Finding {
	out := make([]Finding, len(in.Constraints))
	for i, constraint := range in.Constraints {
		out[i] = Finding{ConstraintID: constraint.ID, Verdict: VerdictUnsure, UnsureCause: cause}
	}
	return out
}
