package review

import (
	"context"
	"sort"
	"sync"
)

// Memory is the in-memory queue used by tests. It applies the same decision
// and sample rules as the postgres adapter. It never draws a new pass sample
// while listing.
type Memory struct {
	mu       sync.Mutex
	users    map[string]string
	labs     map[string]memoryLab
	members  map[string]map[string]bool
	findings map[string]*memoryFinding
	replays  map[string]memoryReplay
}

type memoryLab struct {
	ClassID    string
	Processing bool
}

type memoryFinding struct {
	Finding   Finding
	Seq       int64
	Pos       int
	Active    bool
	Decisions int
}

type memoryReplay struct {
	Fingerprint string
	Result      DecisionResult
}

// NewMemory returns an empty queue.
func NewMemory() *Memory {
	return &Memory{
		users:    map[string]string{},
		labs:     map[string]memoryLab{},
		members:  map[string]map[string]bool{},
		findings: map[string]*memoryFinding{},
		replays:  map[string]memoryReplay{},
	}
}

// PutUser records the role stored for a user.
func (m *Memory) PutUser(id, role string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[id] = role
}

// PutLab records a lab and whether judging or checking is still unfinished.
func (m *Memory) PutLab(id, classID string, processing bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.labs[id] = memoryLab{ClassID: classID, Processing: processing}
}

// PutMember marks a current class membership.
func (m *Memory) PutMember(classID, userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.members[classID] == nil {
		m.members[classID] = map[string]bool{}
	}
	m.members[classID][userID] = true
}

// RemoveMember drops a current class membership.
func (m *Memory) RemoveMember(classID, userID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.members[classID], userID)
}

// PutFinding stores one finding. active is false for a superseded revision.
func (m *Memory) PutFinding(finding Finding, seq int64, pos int, active bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if finding.Evidence == nil {
		finding.Evidence = []Evidence{}
	}
	m.findings[finding.ID] = &memoryFinding{
		Finding: cloneFinding(finding),
		Seq:     seq,
		Pos:     pos,
		Active:  active,
	}
}

// DecisionCount is the number of committed decisions for a finding.
func (m *Memory) DecisionCount(findingID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	row := m.findings[findingID]
	if row == nil {
		return 0
	}
	return row.Decisions
}

// Access implements Store.
func (m *Memory) Access(_ context.Context, labID, userID string) (Access, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	role, userOK := m.users[userID]
	lab, labOK := m.labs[labID]
	access := Access{UserExists: userOK, LabExists: labOK, Role: role}
	if userOK && labOK {
		access.Member = m.members[lab.ClassID][userID]
	}
	return access, nil
}

// List implements Store. It returns the stored RequiredReview flags.
func (m *Memory) List(_ context.Context, labID string, limit int, cursor string) (Page, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	lab, ok := m.labs[labID]
	if !ok {
		return Page{}, ErrNotFound
	}
	var startSeq int64
	var startPos int
	var startID string
	useCursor := cursor != ""
	if useCursor {
		var err error
		startSeq, startPos, startID, err = DecodeCursor(cursor)
		if err != nil {
			return Page{}, err
		}
	}
	rows := make([]*memoryFinding, 0, len(m.findings))
	pending := 0
	for _, row := range m.findings {
		if row.Finding.LabID != labID || !row.Active {
			continue
		}
		if row.Finding.RequiredReview && row.Finding.Decision == nil {
			pending++
		}
		if useCursor && !afterCursor(row, startSeq, startPos, startID) {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Seq != rows[j].Seq {
			return rows[i].Seq < rows[j].Seq
		}
		if rows[i].Pos != rows[j].Pos {
			return rows[i].Pos < rows[j].Pos
		}
		return rows[i].Finding.ID < rows[j].Finding.ID
	})
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	items := make([]Finding, 0, len(rows))
	for _, row := range rows {
		items = append(items, cloneFinding(row.Finding))
	}
	page := Page{Items: items, ProcessingPending: lab.Processing, ReviewPending: pending}
	if more && len(rows) > 0 {
		last := rows[len(rows)-1]
		encoded := EncodeCursor(last.Seq, last.Pos, last.Finding.ID)
		page.NextCursor = &encoded
	}
	return page, nil
}

func afterCursor(row *memoryFinding, seq int64, pos int, id string) bool {
	if row.Seq != seq {
		return row.Seq > seq
	}
	if row.Pos != pos {
		return row.Pos > pos
	}
	return row.Finding.ID > id
}

// Decide implements Store.
func (m *Memory) Decide(_ context.Context, req DecisionRequest) (DecisionResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if req.IdempotencyKey == "" || req.Fingerprint == "" {
		return DecisionResult{}, ErrInvalid
	}
	role, userOK := m.users[req.ActorID]
	lab, labOK := m.labs[req.LabID]
	access := Access{UserExists: userOK, LabExists: labOK, Role: role}
	if userOK && labOK {
		access.Member = m.members[lab.ClassID][req.ActorID]
	}
	if !userOK {
		return DecisionResult{}, ErrNotFound
	}
	if !Allowed(access) {
		return DecisionResult{}, ErrNotAssigned
	}
	key := req.ActorID + "\x00" + req.LabID + "\x00" + req.FindingID + "\x00" + req.IdempotencyKey
	if saved, ok := m.replays[key]; ok {
		if saved.Fingerprint != req.Fingerprint {
			return DecisionResult{Mismatch: true}, nil
		}
		replayed := saved.Result
		replayed.Replay = true
		replayed.Finding = cloneFinding(saved.Result.Finding)
		if saved.Result.Stale != nil {
			copy := *saved.Result.Stale
			replayed.Stale = &copy
		}
		return replayed, nil
	}
	row := m.findings[req.FindingID]
	if row == nil || !row.Active || row.Finding.LabID != req.LabID {
		return DecisionResult{}, ErrNotFound
	}
	next, err := ApplyDecision(row.Finding, req)
	if stale, ok := err.(*StaleRevision); ok {
		result := DecisionResult{Stale: &StaleRevision{Expected: stale.Expected, Current: stale.Current}}
		m.replays[key] = memoryReplay{Fingerprint: req.Fingerprint, Result: result}
		return result, nil
	}
	if err != nil {
		return DecisionResult{}, err
	}
	row.Finding = cloneFinding(next)
	row.Decisions++
	result := DecisionResult{Finding: cloneFinding(next)}
	m.replays[key] = memoryReplay{Fingerprint: req.Fingerprint, Result: result}
	return result, nil
}
