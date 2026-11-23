package postgres

import (
	"context"
	"errors"
	"fmt"
)

// GetLabForClass reads one lab only when it belongs to the supplied class.
// A lab in another class is indistinguishable from an absent lab.
func (s *Store) GetLabForClass(ctx context.Context, labID, classID string) (LabRecord, error) {
	lab, err := s.GetLab(ctx, labID)
	if err != nil {
		if errors.Is(err, ErrLabNotFound) {
			return LabRecord{}, ErrLabNotFound
		}
		return LabRecord{}, err
	}
	if lab.ClassID != classID {
		return LabRecord{}, ErrLabNotFound
	}
	return lab, nil
}

// ListLabsByClass returns safe lab headers for one class in a stable
// newest-first order. Constraints and tests are not loaded for listings.
func (s *Store) ListLabsByClass(ctx context.Context, classID string, limit, offset int) ([]LabRecord, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, class_id, title, statement, opens_at, deadline, revision,
			cpu_seconds, memory_mib, model_notice, created_by
		FROM labs
		WHERE class_id = $1
		ORDER BY created_at DESC, id ASC
		LIMIT $2 OFFSET $3`, classID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("postgres: list labs: %w", err)
	}
	defer rows.Close()

	labs := make([]LabRecord, 0, limit)
	for rows.Next() {
		var lab LabRecord
		if err := rows.Scan(&lab.ID, &lab.ClassID, &lab.Title, &lab.Statement, &lab.OpensAt,
			&lab.Deadline, &lab.Revision, &lab.CPUSeconds, &lab.MemoryMiB, &lab.ModelNotice,
			&lab.CreatedBy,
		); err != nil {
			return nil, fmt.Errorf("postgres: scan lab: %w", err)
		}
		labs = append(labs, lab)
	}
	return labs, rows.Err()
}
