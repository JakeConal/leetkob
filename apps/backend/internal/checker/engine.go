package checker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/JakeConal/leetkob/backend/internal/analyzer"
	"github.com/JakeConal/leetkob/backend/internal/budget"
	"github.com/JakeConal/leetkob/backend/internal/model"
	"github.com/JakeConal/leetkob/backend/internal/sanitize"
)

// Environment variables selected by the marking command. MODEL_OFF is read
// first; the remaining names are only consulted when model use is enabled.
const (
	// EnvAnalyzerPath points at the analyzer executable bundled in the
	// checker image. Without it no hard-rule pair is guessed: every
	// ban-list pair becomes unsure with an explicit cause.
	EnvAnalyzerPath = "CHECKER_ANALYZER"

	// EnvInputPrice and EnvOutputPrice are verified per-1K-token prices in
	// integer cents. An absent or zero price leaves the price schedule
	// unknown, which refuses model dispatch instead of charging zero.
	EnvInputPrice  = "MODEL_INPUT_CENTS_PER_1K"
	EnvOutputPrice = "MODEL_OUTPUT_CENTS_PER_1K"

	// EnvLabID identifies the spend bucket for one command run. The
	// command's in-memory store is intentionally run-scoped.
	EnvLabID = "MODEL_LAB_ID"
)

// Unsure causes recorded in the run summary and logs. They are never written
// into the prediction JSONL, whose schema only carries solution, constraint,
// verdict and original-line evidence.
const (
	CauseModelOff            = "model-off"
	CauseSanitizeUnsafe      = "sanitize-unsafe"
	CauseModelNotConfigured  = "model-not-configured"
	CauseBudgetUnavailable   = "budget-unavailable"
	CauseBudgetUnknownPrice  = "budget-unknown-price"
	CauseBudgetCapExceeded   = "budget-cap-exceeded"
	CauseBudgetFailed        = "budget-reservation-failed"
	CauseSemanticFailed      = "semantic-check-failed"
	CauseSemanticIncomplete  = "semantic-result-incomplete"
	CauseEvidenceInvalid     = "evidence-invalid"
	CauseHardRuleUnavailable = "hard-rule-unavailable"
	CauseBanListWithoutNames = "ban-list-without-names"
	CauseHardRuleEvidenceBad = "hard-rule-evidence-invalid"
	CauseModelOffAfterSelect = "model-off-selected"
)

// Budget is the small reservation surface the checker needs from
// internal/budget. *budget.Service implements it. A nil Budget refuses
// dispatch: no semantic pair is ever decided without a successful priced
// reservation.
type Budget interface {
	ReserveForCall(ctx context.Context, labID string, maxInputTokens, maxOutputTokens int) (budget.Reservation, error)
	Settle(ctx context.Context, reservation budget.Reservation, usage budget.Usage, providerRequestID string) error
	Release(ctx context.Context, reservation budget.Reservation) error
}

// Options wires the shared checker core. The zero value is offline and
// transport-free: it decides nothing semantically, guesses no hard rule and
// opens no network. A deployment supplies the analyzer, the semantic port and
// the budget service explicitly.
type Options struct {
	// ModelOff is the single offline selection. When true the engine never
	// consults Semantic and never reserves or settles budget, so no
	// provider client or credential can exist behind this value.
	ModelOff bool
	// Analyzer decides ban-list constraints. A nil analyzer yields unsure
	// for every ban-list pair rather than a guessed verdict.
	Analyzer analyzer.Analyzer
	// Semantic is the guarded model port. It is only called for semantic
	// constraints when ModelOff is false and a priced reservation succeeds.
	Semantic model.SemanticChecker
	// Budget reserves conservative spend before every semantic call.
	Budget Budget
	// LabID names the spend bucket. Empty selects "checker".
	LabID string
	// MaxOutputTokens bounds the provider output and the reservation.
	// Zero selects model.DefaultMaxOutputTokens.
	MaxOutputTokens int
	// Identities are known student account strings removed by the
	// sanitizer before any semantic transmission.
	Identities []string
}

// OptionsFromEnv selects the command's dependencies in the contract order:
// MODEL_OFF first, then the bundled analyzer, then (only when model use is
// enabled) non-secret provider configuration and a priced budget service.
// No credential is read here; model.NewHTTPChecker reads the key lazily on
// the first real dispatch.
func OptionsFromEnv() Options {
	options := Options{
		ModelOff:        ModelOff(),
		Analyzer:        analyzerFromEnv(),
		LabID:           strings.TrimSpace(os.Getenv(EnvLabID)),
		MaxOutputTokens: model.DefaultMaxOutputTokens,
	}
	if options.ModelOff {
		// Offline selection happens before any provider configuration is
		// loaded. Hard rules still run; every semantic pair is unsure.
		return options
	}
	semantic, spend, maxOutputTokens := semanticFromEnv()
	options.Semantic = semantic
	options.Budget = spend
	if maxOutputTokens > 0 {
		options.MaxOutputTokens = maxOutputTokens
	}
	return options
}

func analyzerFromEnv() analyzer.Analyzer {
	path := strings.TrimSpace(os.Getenv(EnvAnalyzerPath))
	if path == "" {
		return nil
	}
	return NewExecAnalyzer(path)
}

// semanticFromEnv constructs the model port and a priced, run-scoped budget
// service. Any failure leaves the port nil so the engine records an honest
// unsure result instead of dispatching under guessed configuration.
func semanticFromEnv() (model.SemanticChecker, Budget, int) {
	cfg, err := model.ConfigFromEnv()
	if err != nil {
		return nil, nil, 0
	}
	checker, err := model.NewHTTPChecker(cfg)
	if err != nil {
		return nil, nil, 0
	}
	service := budget.NewService(newMemoryBudgetStore(), budget.SystemClock{}, pricesFromEnv())
	return checker, service, cfg.MaxOutputTokens
}

func pricesFromEnv() budget.PriceSchedule {
	return budget.PriceSchedule{
		InputCentsPer1K:  envNonNegativeInt(EnvInputPrice),
		OutputCentsPer1K: envNonNegativeInt(EnvOutputPrice),
	}
}

func envNonNegativeInt(name string) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0
	}
	value := 0
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0
		}
		value = value*10 + int(r-'0')
		if value > 1_000_000 {
			return 0
		}
	}
	return value
}

// decision is the internal verdict for one solution/constraint pair. The
// cause never reaches the prediction file; it only feeds the run summary so
// an operator can see why pairs stayed unsure.
type decision struct {
	verdict  Verdict
	evidence []Evidence
	cause    string
}

func unsureDecision(cause string) decision {
	return decision{verdict: VerdictUnsure, cause: cause}
}

// chooseDecision is the shared rule: ban-list constraints use hard rules
// only, semantic constraints use the model only behind a priced reservation.
// A missing decision is never guessed.
func chooseDecision(constraint Constraint, hard, semantic map[string]decision) decision {
	if IsBanListConstraint(constraint.Text) {
		if decided, ok := hard[constraint.ID]; ok {
			return decided
		}
		return unsureDecision(CauseBanListWithoutNames)
	}
	if decided, ok := semantic[constraint.ID]; ok {
		return decided
	}
	return unsureDecision(CauseSemanticIncomplete)
}

// decideHardRules groups every banned name for one solution into bounded
// analyzer batches. The analyzer decides syntax and resolved declarations,
// so comments, strings and student-defined names cannot become a deduction.
func (e *engine) decideHardRules(ctx context.Context, source string, constraints []Constraint) map[string]decision {
	banLists := make([]Constraint, 0, len(constraints))
	for _, constraint := range constraints {
		if IsBanListConstraint(constraint.Text) {
			banLists = append(banLists, constraint)
		}
	}
	if len(banLists) == 0 {
		return nil
	}

	decisions := make(map[string]decision, len(banLists))
	for _, constraint := range banLists {
		if len(constraint.BannedNames) == 0 {
			decisions[constraint.ID] = unsureDecision(CauseBanListWithoutNames)
		}
	}

	report, cause := e.analyzeBannedNames(ctx, source, banLists)
	if cause != "" {
		for _, constraint := range banLists {
			if len(constraint.BannedNames) == 0 {
				continue
			}
			decisions[constraint.ID] = unsureDecision(cause)
		}
		return decisions
	}

	byName := make(map[string]analyzer.Result, len(report.Results))
	for _, result := range report.Results {
		byName[result.Name] = result
	}
	lines := sourceLines(source)
	for _, constraint := range banLists {
		if len(constraint.BannedNames) == 0 {
			continue
		}
		verdict := VerdictPass
		cause := ""
		var evidence []Evidence
		for _, name := range constraint.BannedNames {
			result, ok := byName[name]
			if !ok {
				verdict = VerdictUnsure
				cause = CauseSemanticIncomplete
				continue
			}
			switch result.Verdict {
			case analyzer.VerdictViolation:
				verdict = VerdictViolation
				mapped, ok := mapHardRuleEvidence(result.Evidence, lines)
				if !ok {
					verdict = VerdictUnsure
					cause = CauseHardRuleEvidenceBad
					continue
				}
				evidence = append(evidence, mapped...)
			case analyzer.VerdictPass:
				// A pass never overrides another name's violation.
			default:
				if verdict != VerdictViolation {
					verdict = VerdictUnsure
					cause = CauseHardRuleUnavailable
				}
			}
		}
		if verdict == VerdictViolation {
			decisions[constraint.ID] = decision{
				verdict:  VerdictViolation,
				evidence: boundEvidence(evidence),
			}
			continue
		}
		if verdict == VerdictUnsure {
			decisions[constraint.ID] = unsureDecision(cause)
			continue
		}
		decisions[constraint.ID] = decision{verdict: VerdictPass}
	}
	return decisions
}

// analyzeBannedNames calls the analyzer once per bounded batch of unique
// banned names. A tooling failure returns a cause and no partial verdicts.
func (e *engine) analyzeBannedNames(ctx context.Context, source string, constraints []Constraint) (analyzer.Report, string) {
	if e.options.Analyzer == nil {
		return analyzer.Report{}, CauseHardRuleUnavailable
	}
	names := make([]string, 0, analyzer.MaxBannedNames)
	seen := make(map[string]bool)
	for _, constraint := range constraints {
		for _, name := range constraint.BannedNames {
			if seen[name] {
				continue
			}
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return analyzer.Report{}, CauseBanListWithoutNames
	}

	var results []analyzer.Result
	for start := 0; start < len(names); start += analyzer.MaxBannedNames {
		end := start + analyzer.MaxBannedNames
		if end > len(names) {
			end = len(names)
		}
		if err := ctx.Err(); err != nil {
			return analyzer.Report{}, CauseHardRuleUnavailable
		}
		report, err := e.options.Analyzer.Analyze(ctx, analyzer.Request{
			Source:      source,
			BannedNames: names[start:end],
		})
		if err != nil {
			return analyzer.Report{}, CauseHardRuleUnavailable
		}
		results = append(results, report.Results...)
	}
	return analyzer.Report{Version: analyzer.ProtocolVersion, Results: results}, ""
}

// mapHardRuleEvidence converts analyzer evidence into prediction evidence.
// The analyzer validates against the same source, so this only re-checks the
// line bound and uses the original line text.
func mapHardRuleEvidence(evidence []analyzer.Evidence, lines []string) ([]Evidence, bool) {
	mapped := make([]Evidence, 0, len(evidence))
	for _, item := range evidence {
		if item.Line < 1 || item.Line > len(lines) {
			return nil, false
		}
		text := lines[item.Line-1]
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		mapped = append(mapped, Evidence{Line: item.Line, Text: text})
	}
	if len(mapped) == 0 {
		return nil, false
	}
	return mapped, true
}

// decideSemantic decides every non-ban-list constraint. It returns the
// decision per constraint ID plus whether a model dispatch actually
// happened, which the summary reports as call evidence.
func (e *engine) decideSemantic(ctx context.Context, statement, source string, constraints []Constraint) (map[string]decision, bool) {
	semantic := make([]Constraint, 0, len(constraints))
	for _, constraint := range constraints {
		if !IsBanListConstraint(constraint.Text) {
			semantic = append(semantic, constraint)
		}
	}
	if len(semantic) == 0 {
		return nil, false
	}

	fail := func(cause string) map[string]decision {
		decisions := make(map[string]decision, len(semantic))
		for _, constraint := range semantic {
			decisions[constraint.ID] = unsureDecision(cause)
		}
		return decisions
	}

	if e.options.ModelOff {
		// Selection happens before any provider client or credential
		// read. No transport method is reachable behind this branch.
		return fail(CauseModelOff), false
	}

	sanitizedSource, err := sanitize.Source(source, e.options.Identities)
	if err != nil || sanitizedSource.Unsafe {
		return fail(CauseSanitizeUnsafe), false
	}
	sanitizedStatement, err := sanitize.Text(statement, e.options.Identities)
	if err != nil || sanitizedStatement.Unsafe {
		return fail(CauseSanitizeUnsafe), false
	}
	if e.options.Semantic == nil {
		return fail(CauseModelNotConfigured), false
	}
	if e.options.Budget == nil {
		return fail(CauseBudgetUnavailable), false
	}

	request := model.Request{
		Statement:   sanitizedStatement.Source,
		Source:      sanitizedSource.Source,
		Constraints: make([]model.Constraint, 0, len(semantic)),
	}
	for _, constraint := range semantic {
		text, err := sanitize.Text(constraint.Text, e.options.Identities)
		if err != nil || text.Unsafe {
			return fail(CauseSanitizeUnsafe), false
		}
		request.Constraints = append(request.Constraints, model.Constraint{ID: constraint.ID, Text: text.Source})
	}

	maxOutputTokens := e.options.MaxOutputTokens
	if maxOutputTokens <= 0 {
		maxOutputTokens = model.DefaultMaxOutputTokens
	}
	// The bound covers the first dispatch plus at most one repair retry,
	// the system prompt and the bounded echo of a rejected response. A
	// token is at least one byte, so byte length is a conservative maximum.
	maxInputTokens := 2*(len(request.Statement)+len(request.Source)+len(request.Constraints)*256) + 8192
	labID := e.options.LabID
	if labID == "" {
		labID = "checker"
	}
	reservation, err := e.options.Budget.ReserveForCall(ctx, labID, maxInputTokens, 2*maxOutputTokens)
	if err != nil {
		switch {
		case errors.Is(err, budget.ErrUnknownPrice):
			return fail(CauseBudgetUnknownPrice), false
		case errors.Is(err, budget.ErrCapExceeded):
			return fail(CauseBudgetCapExceeded), false
		default:
			return fail(CauseBudgetFailed), false
		}
	}

	result, checkErr := e.options.Semantic.Check(ctx, request)
	if checkErr != nil {
		// No provider usage is known after a pre-dispatch refusal or a
		// transport error owned by the port; release rather than charge.
		_ = e.options.Budget.Release(ctx, reservation)
		if errors.Is(checkErr, model.ErrModelOff) {
			return fail(CauseModelOffAfterSelect), false
		}
		return fail(CauseSemanticFailed), false
	}

	usage := budget.Usage{
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
		Known:        result.Attempts > 0 && (result.InputTokens > 0 || result.OutputTokens > 0),
	}
	if err := e.options.Budget.Settle(ctx, reservation, usage, result.ProviderRequestID); err != nil {
		// Settlement failure does not change the verdict; the reservation
		// remains charged by the durable store's contract.
	}

	lines := sanitizedLines(source)
	byID := make(map[string]model.ConstraintResult, len(result.Constraints))
	for _, constraint := range result.Constraints {
		byID[constraint.ConstraintID] = constraint
	}
	decisions := make(map[string]decision, len(semantic))
	for _, constraint := range semantic {
		decided, ok := byID[constraint.ID]
		if !ok {
			decisions[constraint.ID] = unsureDecision(CauseSemanticIncomplete)
			continue
		}
		switch decided.Verdict {
		case model.VerdictPass:
			decisions[constraint.ID] = decision{verdict: VerdictPass}
		case model.VerdictViolation:
			evidence, ok := mapSemanticEvidence(decided.Evidence, lines)
			if !ok {
				decisions[constraint.ID] = unsureDecision(CauseEvidenceInvalid)
				continue
			}
			decisions[constraint.ID] = decision{verdict: VerdictViolation, evidence: evidence}
		case model.VerdictUnsure:
			cause := CauseSemanticFailed
			if decided.Cause != "" {
				cause = string(decided.Cause)
			}
			decisions[constraint.ID] = unsureDecision(cause)
		default:
			decisions[constraint.ID] = unsureDecision(CauseSemanticIncomplete)
		}
	}
	return decisions, true
}

// mapSemanticEvidence maps a validated model citation back to the original
// source line. Line numbers survive sanitization because comments are
// replaced with spaces, never newlines.
func mapSemanticEvidence(evidence []model.Evidence, lines []string) ([]Evidence, bool) {
	mapped := make([]Evidence, 0, len(evidence))
	for _, item := range evidence {
		if item.Line < 1 || item.Line > len(lines) {
			return nil, false
		}
		text := lines[item.Line-1]
		if strings.TrimSpace(text) == "" {
			return nil, false
		}
		mapped = append(mapped, Evidence{Line: item.Line, Text: text})
	}
	if len(mapped) == 0 {
		return nil, false
	}
	return boundEvidence(mapped), true
}

// sourceLines keeps one-based original line mapping and removes a trailing
// CR from CRLF sources.
func sourceLines(source string) []string {
	parts := strings.Split(source, "\n")
	for i, line := range parts {
		parts[i] = strings.TrimSuffix(line, "\r")
	}
	return parts
}

// sanitizedLines mirrors model's line numbering, which removes a single
// trailing newline before splitting.
func sanitizedLines(source string) []string {
	normalized := strings.ReplaceAll(source, "\r\n", "\n")
	normalized = strings.TrimSuffix(normalized, "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func boundEvidence(evidence []Evidence) []Evidence {
	seen := make(map[int]int, len(evidence))
	unique := make([]Evidence, 0, len(evidence))
	for _, item := range evidence {
		if index, ok := seen[item.Line]; ok {
			if item.Text < unique[index].Text {
				unique[index] = item
			}
			continue
		}
		seen[item.Line] = len(unique)
		unique = append(unique, item)
	}
	sort.Slice(unique, func(i, j int) bool {
		if unique[i].Line != unique[j].Line {
			return unique[i].Line < unique[j].Line
		}
		return unique[i].Text < unique[j].Text
	})
	if len(unique) > analyzer.MaxEvidencePerResult {
		unique = unique[:analyzer.MaxEvidencePerResult]
	}
	return unique
}

// engine carries validated options for one run.
type engine struct {
	options Options
}

func newEngine(options Options) *engine {
	if options.MaxOutputTokens <= 0 {
		options.MaxOutputTokens = model.DefaultMaxOutputTokens
	}
	return &engine{options: options}
}

// evaluateSolution reads one statement and produces every prediction for one
// solution in deterministic constraint order.
func (e *engine) evaluateSolution(ctx context.Context, solution Solution, constraints []Constraint, statement, source string, summary *Summary) ([]Prediction, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("checker: run cancelled: %w", err)
	}
	hard := e.decideHardRules(ctx, source, constraints)
	semantic, dispatched := e.decideSemantic(ctx, statement, source, constraints)
	if dispatched {
		summary.ModelCalls++
	}

	predictions := make([]Prediction, 0, len(constraints))
	for _, constraint := range constraints {
		decided := chooseDecision(constraint, hard, semantic)
		switch decided.verdict {
		case VerdictPass:
			summary.Pass++
		case VerdictViolation:
			summary.Violation++
		default:
			summary.Unsure++
			if summary.UnsureCauses == nil {
				summary.UnsureCauses = make(map[string]int)
			}
			cause := decided.cause
			if cause == "" {
				cause = "unspecified"
			}
			summary.UnsureCauses[cause]++
		}
		predictions = append(predictions, Prediction{
			Solution:   solution.ID,
			Constraint: constraint.ID,
			Verdict:    decided.verdict,
			Evidence:   decided.evidence,
		})
	}
	return predictions, nil
}
