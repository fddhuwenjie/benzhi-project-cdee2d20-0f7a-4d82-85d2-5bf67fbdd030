package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

type transaction struct{ tx *sql.Tx }

func (s *Store) Execute(ctx context.Context, missionID, requestID, operation, fingerprint string, fn func(mission.Tx) (mission.StoredResult, error)) (mission.StoredResult, bool, error) {
	lockKey := "__mission_transactions__"
	lock := s.missionLock(lockKey)
	if err := lock.Lock(ctx); err != nil {
		return mission.StoredResult{}, false, err
	}
	defer lock.Unlock()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return mission.StoredResult{}, false, err
	}
	defer tx.Rollback()
	var existing mission.StoredResult
	var existingOperation string
	var existingFingerprint string
	err = tx.QueryRowContext(ctx, `SELECT operation, payload_fingerprint, status_code, response_body FROM command_results WHERE request_id = ?`, requestID).
		Scan(&existingOperation, &existingFingerprint, &existing.StatusCode, &existing.Body)
	if err == nil {
		if existingOperation != operation || existingFingerprint != "" && existingFingerprint != fingerprint {
			return mission.StoredResult{}, false, mission.NewError("idempotency_key_reused", "request_id 已绑定到不同载荷", 409)
		}
		return existing, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return mission.StoredResult{}, false, err
	}
	result, err := fn(&transaction{tx: tx})
	if err != nil {
		return mission.StoredResult{}, false, err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO command_results(request_id, mission_id, operation, payload_fingerprint, status_code, response_body, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		requestID, missionID, operation, fingerprint, result.StatusCode, []byte(result.Body), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return mission.StoredResult{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return mission.StoredResult{}, false, err
	}
	return result, false, nil
}
