package checker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Dataset and file limits. The source limit is the documented PA#3 submission
// bound (65,536 source bytes); discovery refuses larger files instead of
// loading them, which keeps memory bounded.
const (
	maxSourceBytes       = 65536
	maxStatementBytes    = 65536
	maxConstraintsBytes  = 1 << 20
	maxConstraintTextLen = 4096
	maxIdentifierLen     = 256
)

// Discovery errors. The command maps every dataset problem to exit code 2
// (invalid input) rather than pretending a partial run is valid.
var (
	ErrDatasetEmpty              = errors.New("checker: dataset contains no problem directories")
	ErrProblemMissingStatement   = errors.New("checker: problem directory has no statement file")
	ErrProblemMissingConstraints = errors.New("checker: problem directory has no constraints file")
	ErrProblemHasNoSolutions     = errors.New("checker: problem directory has no solution sources")
	ErrDuplicateProblem          = errors.New("checker: duplicate problem identifier")
	ErrDuplicateSolution         = errors.New("checker: duplicate solution identifier")
	ErrNoConstraints             = errors.New("checker: constraints file contains no constraints")
	ErrInvalidConstraint         = errors.New("checker: invalid constraint")
	ErrSourceTooLarge            = errors.New("checker: solution source exceeds the documented limit")
	ErrInvalidSource             = errors.New("checker: solution source is not valid UTF-8")
	ErrStatementTooLarge         = errors.New("checker: statement exceeds the documented limit")
	ErrInvalidStatement          = errors.New("checker: statement is not valid UTF-8")
	ErrConstraintsTooLarge       = errors.New("checker: constraints file exceeds the bounded input limit")
)

// Constraint is one discovered constraint. Ban-list texts are decided by
// the hard-rule analyzer; semantic texts are unsure in the offline command,
// which never calls a model.
type Constraint struct {
	ID   string
	Text string
}

// Solution is one discovered solution source. ID is the dataset-level label
// identifier "<problem>/<file-base>", matching the upstream label format.
type Solution struct {
	ID       string
	FileName string
	Path     string
}

// Problem is one discovered problem or lab directory.
type Problem struct {
	ID              string
	Dir             string
	StatementPath   string
	ConstraintsPath string
	Constraints     []Constraint
	Solutions       []Solution
}

// fileOps is the narrow filesystem surface used by discovery. Tests inject a
// recording wrapper to prove that labels, scorer and test files are never
// opened, not merely unread.
type fileOps struct {
	readDir  func(string) ([]os.DirEntry, error)
	readFile func(string) ([]byte, error)
	stat     func(string) (os.FileInfo, error)
}

var defaultFileOps = fileOps{
	readDir:  os.ReadDir,
	readFile: os.ReadFile,
	stat:     os.Stat,
}

// containerDirs are dataset-level container names that are never problems
// themselves. "solutions" is also the alternate top-level solution root.
var containerDirs = map[string]bool{
	"problems":    true,
	"labs":        true,
	"solutions":   true,
	"tests":       true,
	"team-review": true,
	"labels":      true,
}

var statementExts = map[string]bool{
	"":     true,
	".md":  true,
	".txt": true,
}

var constraintsExts = map[string]bool{
	"":      true,
	".json": true,
	".txt":  true,
}

var solutionExts = map[string]bool{
	".cpp": true,
	".cc":  true,
	".cxx": true,
}

// extensionRank makes candidate selection deterministic when several
// documented names coexist: exact base name, then .md/.json, then .txt, then
// lexicographic.
func extensionRank(ext string) int {
	switch strings.ToLower(ext) {
	case "":
		return 0
	case ".md", ".json":
		return 1
	case ".txt":
		return 2
	default:
		return 3
	}
}

// Discover returns the problems found under a dataset root using only the
// documented layout. It discovers statement paths, parses constraints and
// collects solution metadata; labels, scorer programs, tests and other
// metadata are never opened.
func Discover(root string) ([]Problem, error) {
	return discoverDataset(root, defaultFileOps)
}

func discoverDataset(root string, ops fileOps) ([]Problem, error) {
	root = filepath.Clean(root)

	roots := []string{root}
	for _, container := range []string{"problems", "labs"} {
		dir := filepath.Join(root, container)
		if info, err := ops.stat(dir); err == nil && info.IsDir() {
			roots = append(roots, dir)
		}
	}

	seen := make(map[string]bool)
	var problems []Problem
	for _, problemRoot := range roots {
		entries, err := ops.readDir(problemRoot)
		if err != nil {
			return nil, fmt.Errorf("checker: read dataset directory %s: %w", problemRoot, err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
				continue
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") || containerDirs[name] {
				continue
			}
			dir := filepath.Join(problemRoot, name)
			statementPath, hasStatement, err := findNamedFile(dir, "statement", statementExts, ops)
			if err != nil {
				return nil, err
			}
			constraintsPath, hasConstraints, err := findNamedFile(dir, "constraints", constraintsExts, ops)
			if err != nil {
				return nil, err
			}
			if !hasStatement && !hasConstraints {
				continue
			}
			if seen[name] {
				return nil, fmt.Errorf("%w: %s", ErrDuplicateProblem, name)
			}
			seen[name] = true

			problem := Problem{
				ID:              name,
				Dir:             dir,
				StatementPath:   statementPath,
				ConstraintsPath: constraintsPath,
			}
			if problem.StatementPath == "" {
				return nil, fmt.Errorf("%w: %s", ErrProblemMissingStatement, name)
			}
			if problem.ConstraintsPath == "" {
				return nil, fmt.Errorf("%w: %s", ErrProblemMissingConstraints, name)
			}

			solutions, err := discoverSolutions(root, problem.ID, problem.Dir, ops)
			if err != nil {
				return nil, err
			}
			if len(solutions) == 0 {
				return nil, fmt.Errorf("%w: %s", ErrProblemHasNoSolutions, name)
			}
			problem.Solutions = solutions

			if err := loadProblemConstraints(&problem, ops); err != nil {
				return nil, err
			}
			problems = append(problems, problem)
		}
	}

	if len(problems) == 0 {
		return nil, ErrDatasetEmpty
	}
	sort.Slice(problems, func(i, j int) bool { return problems[i].ID < problems[j].ID })
	return problems, nil
}

// findNamedFile returns the preferred "<base>" or "<base><ext>" regular file in
// dir. A base name may carry an extra suffix, for example
// "statement-final.md"; the documented extension still has to match.
func findNamedFile(dir, base string, exts map[string]bool, ops fileOps) (string, bool, error) {
	entries, err := ops.readDir(dir)
	if err != nil {
		return "", false, fmt.Errorf("checker: read problem directory %s: %w", dir, err)
	}
	type candidate struct {
		name string
		rank int
	}
	var candidates []candidate
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		lower := strings.ToLower(name)
		if !strings.HasPrefix(lower, base) {
			continue
		}
		// Require the documented base name or a separated suffix such as
		// "statement-final.md". A bare prefix like "statementless" is not a
		// statement file.
		suffix := lower[len(base):]
		if suffix != "" && !strings.HasPrefix(suffix, ".") &&
			!strings.HasPrefix(suffix, "-") && !strings.HasPrefix(suffix, "_") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(lower))
		if !exts[ext] {
			continue
		}
		candidates = append(candidates, candidate{name: name, rank: extensionRank(ext)})
	}
	if len(candidates) == 0 {
		return "", false, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].rank != candidates[j].rank {
			return candidates[i].rank < candidates[j].rank
		}
		return candidates[i].name < candidates[j].name
	})
	return filepath.Join(dir, candidates[0].name), true, nil
}

func discoverSolutions(root, problemID, problemDir string, ops fileOps) ([]Solution, error) {
	dirs := []string{
		filepath.Join(problemDir, "solutions"),
		filepath.Join(root, "solutions", problemID),
	}
	byID := make(map[string]Solution)
	for _, dir := range dirs {
		info, err := ops.stat(dir)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("checker: stat solutions directory %s: %w", dir, err)
		}
		if !info.IsDir() {
			continue
		}
		entries, err := ops.readDir(dir)
		if err != nil {
			return nil, fmt.Errorf("checker: read solutions directory %s: %w", dir, err)
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&fs.ModeSymlink != 0 {
				continue
			}
			name := entry.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			if !solutionExts[strings.ToLower(filepath.Ext(name))] {
				continue
			}
			base := strings.TrimSuffix(name, filepath.Ext(name))
			id := problemID + "/" + base
			if previous, exists := byID[id]; exists {
				return nil, fmt.Errorf("%w: %s (%s and %s)", ErrDuplicateSolution, id, previous.Path, filepath.Join(dir, name))
			}
			byID[id] = Solution{ID: id, FileName: name, Path: filepath.Join(dir, name)}
		}
	}
	solutions := make([]Solution, 0, len(byID))
	for _, solution := range byID {
		solutions = append(solutions, solution)
	}
	sort.Slice(solutions, func(i, j int) bool { return solutions[i].ID < solutions[j].ID })
	return solutions, nil
}

func loadProblemConstraints(problem *Problem, ops fileOps) error {
	info, err := ops.stat(problem.ConstraintsPath)
	if err != nil {
		return fmt.Errorf("checker: stat constraints %s: %w", problem.ConstraintsPath, err)
	}
	if info.Size() > maxConstraintsBytes {
		return fmt.Errorf("%w: %s", ErrConstraintsTooLarge, problem.ConstraintsPath)
	}
	data, err := ops.readFile(problem.ConstraintsPath)
	if err != nil {
		return fmt.Errorf("checker: read constraints %s: %w", problem.ConstraintsPath, err)
	}
	ext := strings.ToLower(filepath.Ext(problem.ConstraintsPath))
	var constraints []Constraint
	if ext == ".json" || (ext == "" && bytes.HasPrefix(bytes.TrimSpace(data), []byte("{"))) ||
		(ext == "" && bytes.HasPrefix(bytes.TrimSpace(data), []byte("["))) {
		constraints, err = parseConstraintsJSON(data)
	} else {
		constraints, err = parseConstraintLines(data)
	}
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInvalidConstraint, problem.ConstraintsPath, err)
	}
	problem.Constraints = constraints
	return nil
}

type constraintJSON struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

func parseConstraintsJSON(data []byte) ([]Constraint, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, ErrNoConstraints
	}
	var items []constraintJSON
	if trimmed[0] == '[' {
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, fmt.Errorf("invalid constraint JSON array: %w", err)
		}
	} else {
		var wrapper struct {
			Constraints []constraintJSON `json:"constraints"`
		}
		if err := json.Unmarshal(trimmed, &wrapper); err != nil {
			return nil, fmt.Errorf("invalid constraint JSON object: %w", err)
		}
		items = wrapper.Constraints
	}
	if len(items) == 0 {
		return nil, ErrNoConstraints
	}
	constraints := make([]Constraint, 0, len(items))
	seen := make(map[string]bool)
	for index, item := range items {
		id := strings.TrimSpace(item.ID)
		text := strings.TrimSpace(item.Text)
		if id == "" || text == "" || len(id) > maxIdentifierLen || utf8.RuneCountInString(text) > maxConstraintTextLen {
			return nil, fmt.Errorf("%w at index %d: id/text missing or out of bounds", ErrInvalidConstraint, index)
		}
		if seen[id] {
			return nil, fmt.Errorf("%w at index %d: duplicate id %q", ErrInvalidConstraint, index, id)
		}
		seen[id] = true
		constraints = append(constraints, Constraint{ID: id, Text: text})
	}
	return constraints, nil
}

var lineConstraintID = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)

// parseConstraintLines accepts one constraint per line. The identifier is
// optional; without it a stable C<n> identifier is assigned by line order.
func parseConstraintLines(data []byte) ([]Constraint, error) {
	if !utf8.Valid(data) {
		return nil, errors.New("constraints file is not valid UTF-8")
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	var constraints []Constraint
	seen := make(map[string]bool)
	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		id, body := splitConstraintLine(line)
		if body == "" {
			continue
		}
		if id == "" {
			id = fmt.Sprintf("C%d", len(constraints)+1)
		}
		if len(id) > maxIdentifierLen || utf8.RuneCountInString(body) > maxConstraintTextLen {
			return nil, fmt.Errorf("constraint identifier or text out of bounds on line %q", line)
		}
		if seen[id] {
			return nil, fmt.Errorf("duplicate constraint id %q", id)
		}
		seen[id] = true
		constraints = append(constraints, Constraint{ID: id, Text: body})
	}
	if len(constraints) == 0 {
		return nil, ErrNoConstraints
	}
	return constraints, nil
}

func splitConstraintLine(line string) (id, text string) {
	if index := strings.IndexAny(line, ":\t"); index > 0 {
		candidate := strings.TrimSpace(line[:index])
		rest := strings.TrimSpace(line[index+1:])
		if rest != "" && looksLikeConstraintID(candidate) {
			return candidate, rest
		}
	}
	if index := strings.IndexAny(line, " \t"); index > 0 {
		candidate := line[:index]
		rest := strings.TrimSpace(line[index+1:])
		if rest != "" && looksLikeConstraintID(candidate) && strings.ContainsAny(candidate, "0123456789") {
			return candidate, rest
		}
	}
	return "", line
}

// looksLikeConstraintID differs from the lab-creation identifier pattern in one
// way: a candidate with no digits must be all uppercase. That keeps prose lines
// such as "Note: keep it simple" as text with a generated identifier instead of
// silently renaming the constraint "Note", while still accepting "C1", "K2",
// "BAN" and "L1" in line-per-constraint files.
func looksLikeConstraintID(candidate string) bool {
	if !lineConstraintID.MatchString(candidate) || candidate == "" {
		return false
	}
	if strings.ContainsAny(candidate, "0123456789-_") {
		return true
	}
	return candidate == strings.ToUpper(candidate)
}
