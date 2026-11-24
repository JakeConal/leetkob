package checker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/JakeConal/leetkob/backend/internal/checker/hardrule"
	"github.com/JakeConal/leetkob/backend/internal/checker/schema"
	"github.com/JakeConal/leetkob/backend/internal/semantic"
)

// AnalyzeTimeout bounds one native analyzer invocation.
const AnalyzeTimeout = 30 * time.Second

// HardRuleEngine turns one solution source into an analyzer-result.v1
// document. The CLI uses a NativeAnalyzer; tests inject a fake. A nil
// engine means hard rules cannot run, and every ban-list pair becomes
// unsure instead of a guessed verdict.
type HardRuleEngine interface {
	Analyze(ctx context.Context, source string) ([]byte, error)
}

// NativeAnalyzer runs the bundled libclang analyzer binary. The binary is
// built into the checker image; when it is absent the caller must fall back
// to unsure rather than compile on a trusted host.
type NativeAnalyzer struct {
	Binary       string
	CompilerArgs []string
}

// Analyze executes the analyzer against one temporary source file.
func (n *NativeAnalyzer) Analyze(ctx context.Context, source string) ([]byte, error) {
	if n == nil || n.Binary == "" {
		return nil, errors.New("checker: analyzer binary is not configured")
	}
	tempDir, err := os.MkdirTemp("", "leetkob-checker-")
	if err != nil {
		return nil, fmt.Errorf("checker: analyzer temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	sourcePath := filepath.Join(tempDir, "source.cpp")
	if err := os.WriteFile(sourcePath, []byte(source), 0o600); err != nil {
		return nil, fmt.Errorf("checker: analyzer source file: %w", err)
	}
	analyzeCtx, cancel := context.WithTimeout(ctx, AnalyzeTimeout)
	defer cancel()
	return hardrule.RunAnalyzer(analyzeCtx, n.Binary, sourcePath, n.CompilerArgs)
}

// NewNativeAnalyzer wires the analyzer from CHECKER_ANALYZER (a prebuilt
// binary bundled in the image). It returns (nil, nil) when the variable is
// empty so the CLI reports unsure hard-rule outcomes instead of failing.
func NewNativeAnalyzer(ctx context.Context) (*NativeAnalyzer, error) {
	binary := os.Getenv("CHECKER_ANALYZER")
	if binary == "" {
		return nil, nil
	}
	args, err := hardrule.CompilerArgs(ctx)
	if err != nil {
		return nil, fmt.Errorf("checker: compiler include paths: %w", err)
	}
	return &NativeAnalyzer{Binary: binary, CompilerArgs: args}, nil
}

// RunOptions configures one offline marking run.
type RunOptions struct {
	// Root is the dataset directory discovered generically.
	Root string
	// Predictions is the JSONL output path.
	Predictions string
	// Engine runs hard rules; nil yields unsure ban-list verdicts.
	Engine HardRuleEngine
	// Semantic runs semantic constraints; nil yields unsure semantic
	// verdicts. MODEL_OFF runs pass nil here.
	Semantic SemanticEngine
}

// Summary reports one run's bounded counts.
type Summary struct {
	Problems    int
	Solutions   int
	Constraints int
	Predictions int
	Pass        int
	Violation   int
	Unsure      int
}

// Run discovers the dataset, decides every pair the offline core can
// decide and writes one validated JSONL prediction per solution and
// constraint. Ban-list constraints use the hard-rule analyzer; semantic
// constraints are unsure because the offline command never calls a model.
// Labels, tags, tests and scorer files are never read.
func Run(ctx context.Context, options RunOptions) (Summary, error) {
	if options.Root == "" || options.Predictions == "" {
		return Summary{}, errors.New("checker: dataset root and prediction path are required")
	}
	problems, err := Discover(options.Root)
	if err != nil {
		return Summary{}, err
	}

	summary := Summary{Problems: len(problems)}
	predictions := make([]schema.Prediction, 0, 64)
	for _, problem := range problems {
		summary.Constraints += len(problem.Constraints)
		statement, err := readText(problem.StatementPath, maxStatementBytes, ErrStatementTooLarge)
		if err != nil {
			return Summary{}, err
		}
		for _, solution := range problem.Solutions {
			summary.Solutions++
			source, err := readSolutionSource(solution.Path)
			if err != nil {
				return Summary{}, err
			}
			raw, analyzerErr := analyze(ctx, options.Engine, source)

			decisions := make(map[string]hardrule.Decision, len(problem.Constraints))
			semanticConstraints := make([]semantic.Constraint, 0, len(problem.Constraints))
			if analyzerErr == nil {
				for _, constraint := range problem.Constraints {
					decision := hardrule.Evaluate(constraint.Text, source, raw)
					if decision.HardRule {
						decisions[constraint.ID] = decision
					} else {
						semanticConstraints = append(semanticConstraints, semantic.Constraint{ID: constraint.ID, Text: constraint.Text})
					}
				}
			} else {
				for _, constraint := range problem.Constraints {
					if !hardrule.Evaluate(constraint.Text, source, nil).HardRule {
						semanticConstraints = append(semanticConstraints, semantic.Constraint{ID: constraint.ID, Text: constraint.Text})
					}
				}
			}

			semanticFindings := map[string]semantic.Finding{}
			if options.Semantic != nil && len(semanticConstraints) > 0 {
				result, err := options.Semantic.Check(ctx, semantic.Input{
					LabID: problem.ID, Statement: statement,
					Constraints: semanticConstraints, Source: source,
				})
				if err == nil {
					for _, finding := range result.Findings {
						semanticFindings[finding.ConstraintID] = finding
					}
				}
			}

			for _, constraint := range problem.Constraints {
				prediction := schema.Prediction{
					Solution:   solution.ID,
					Constraint: constraint.ID,
					Evidence:   []schema.Evidence{},
				}
				switch {
				case analyzerErr != nil:
					if finding, ok := semanticFindings[constraint.ID]; ok {
						applySemanticFinding(&prediction, finding)
					} else {
						prediction.Verdict = schema.VerdictUnsure
					}
				default:
					if decision, ok := decisions[constraint.ID]; ok {
						prediction.Verdict = decision.Verdict
						prediction.Evidence = decision.Evidence
					} else if finding, ok := semanticFindings[constraint.ID]; ok {
						applySemanticFinding(&prediction, finding)
					} else {
						prediction.Verdict = schema.VerdictUnsure
					}
				}
				if prediction.Evidence == nil {
					prediction.Evidence = []schema.Evidence{}
				}
				if err := schema.ValidatePrediction(prediction); err != nil {
					return Summary{}, fmt.Errorf("checker: %s/%s: %w", prediction.Solution, prediction.Constraint, err)
				}
				predictions = append(predictions, prediction)
				switch prediction.Verdict {
				case schema.VerdictPass:
					summary.Pass++
				case schema.VerdictViolation:
					summary.Violation++
				default:
					summary.Unsure++
				}
			}
		}
	}
	summary.Predictions = len(predictions)
	if err := writePredictions(options.Predictions, predictions); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

// readSolutionSource loads one bounded, valid-UTF-8 solution source.
func readSolutionSource(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("checker: read solution: %w", err)
	}
	if info.Size() > maxSourceBytes {
		return "", fmt.Errorf("%w: %s", ErrSourceTooLarge, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("checker: read solution: %w", err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%w: %s", ErrInvalidSource, path)
	}
	return string(data), nil
}

// applySemanticFinding copies one semantic verdict and its restored
// evidence into a prediction. Any non-violation outcome without evidence
// stays an unsure-compatible empty list.
func applySemanticFinding(prediction *schema.Prediction, finding semantic.Finding) {
	switch finding.Verdict {
	case semantic.VerdictPass:
		prediction.Verdict = schema.VerdictPass
	case semantic.VerdictViolation:
		prediction.Verdict = schema.VerdictViolation
		evidence := make([]schema.Evidence, 0, len(finding.Evidence))
		for _, item := range finding.Evidence {
			evidence = append(evidence, schema.Evidence{Line: item.Line, Text: item.Text})
		}
		prediction.Evidence = evidence
	default:
		prediction.Verdict = schema.VerdictUnsure
	}
}

// readText loads one bounded UTF-8 text file.
func readText(path string, limit int64, tooLarge error) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("checker: read text: %w", err)
	}
	if info.Size() > limit {
		return "", fmt.Errorf("%w: %s", tooLarge, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("checker: read text: %w", err)
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("%w: %s", ErrInvalidStatement, path)
	}
	return string(data), nil
}

// analyze runs the hard-rule engine once per solution. The error is not
// fatal: every ban-list pair becomes unsure for that solution.
func analyze(ctx context.Context, engine HardRuleEngine, source string) ([]byte, error) {
	if engine == nil {
		return nil, errors.New("checker: hard-rule analyzer is not configured")
	}
	raw, err := engine.Analyze(ctx, source)
	if err != nil {
		return nil, err
	}
	if _, err := hardrule.ParseResult(raw); err != nil {
		return nil, err
	}
	return raw, nil
}
