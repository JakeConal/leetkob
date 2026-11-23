package review

import (
	"hash/fnv"
	"sort"
)

// Obligations chooses who must be reviewed. Every violation and every
// unsure is required. Pass pairs contribute a stable ceil(10%) sample.
// The sample is a pure function of the submission id and constraint ids,
// so storing the result and reading it later cannot draw a different set.
func Obligations(submissionID string, pairs []Pair) map[string]bool {
	required := make(map[string]bool, len(pairs))
	passes := make([]rankedPass, 0, len(pairs))
	for _, pair := range pairs {
		if pair.Verdict != "pass" {
			required[pair.ConstraintID] = true
			continue
		}
		required[pair.ConstraintID] = false
		passes = append(passes, rankedPass{
			id:  pair.ConstraintID,
			key: sampleKey(submissionID, pair.ConstraintID),
		})
	}
	if len(passes) == 0 {
		return required
	}
	sort.Slice(passes, func(i, j int) bool {
		if passes[i].key != passes[j].key {
			return passes[i].key < passes[j].key
		}
		return passes[i].id < passes[j].id
	})
	need := (len(passes) + 9) / 10
	for i := 0; i < need; i++ {
		required[passes[i].id] = true
	}
	return required
}

type rankedPass struct {
	id  string
	key uint64
}

func sampleKey(submissionID, constraintID string) uint64 {
	hash := fnv.New64a()
	_, _ = hash.Write([]byte(submissionID))
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write([]byte(constraintID))
	return hash.Sum64()
}
