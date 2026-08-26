package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/mission"
)

type sqlRunner interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (t *transaction) LoadMission(ctx context.Context, id string) (*mission.DiveMission, error) {
	return loadMission(ctx, t.tx, id)
}

func (s *Store) Mission(ctx context.Context, id string) (*mission.DiveMission, error) {
	lock := s.missionLock("__mission_transactions__")
	lock.Lock()
	defer lock.Unlock()
	m, err := loadMission(ctx, s.db, id)
	if err != nil {
		return nil, err
	}
	if m.Status == mission.StatusArchived {
		events, err := s.AllAuditEvents(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := verifyArchive(m, events); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (s *Store) MissionUnverified(ctx context.Context, id string) (*mission.DiveMission, error) {
	lock := s.missionLock("__mission_transactions__")
	lock.Lock()
	defer lock.Unlock()
	return loadMission(ctx, s.db, id)
}

func loadMission(ctx context.Context, q sqlRunner, id string) (*mission.DiveMission, error) {
	m := &mission.DiveMission{ID: id}
	var windowStart, windowEnd, createdAt, updatedAt string
	var archivedAt sql.NullString
	var checklistJSON []byte
	var riskSummaryJSON, rejectionJSON, planHistoryJSON, verificationHistoryJSON, qualificationsJSON, riskHistoryJSON []byte
	err := q.QueryRowContext(ctx, `SELECT title, cave_site, cave_site_key, target_depth_m, window_start, window_end, status, revision, leader_id, release_digest, release_checklist, signed_by, archive_digest, risk_summary, last_release_rejection, created_at, updated_at, archived_at, plan_history, verification_history, template_mission_id, template_archive_digest, qualification_snapshot, risk_history FROM missions WHERE id = ?`, id).
		Scan(&m.Title, &m.CaveSite, &m.CaveSiteKey, &m.TargetDepthM, &windowStart, &windowEnd, &m.Status, &m.Revision, &m.LeaderID, &m.ReleaseDigest, &checklistJSON, &m.SignedBy, &m.ArchiveDigest, &riskSummaryJSON, &rejectionJSON, &createdAt, &updatedAt, &archivedAt, &planHistoryJSON, &verificationHistoryJSON, &m.TemplateMissionID, &m.TemplateArchiveDigest, &qualificationsJSON, &riskHistoryJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mission.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.WindowStart, err = time.Parse(time.RFC3339Nano, windowStart)
	if err != nil {
		return nil, err
	}
	m.WindowEnd, err = time.Parse(time.RFC3339Nano, windowEnd)
	if err != nil {
		return nil, err
	}
	m.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, err
	}
	m.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, err
	}
	if archivedAt.Valid {
		value, parseErr := time.Parse(time.RFC3339Nano, archivedAt.String)
		if parseErr != nil {
			return nil, parseErr
		}
		m.ArchivedAt = &value
	}
	if err := json.Unmarshal(checklistJSON, &m.ReleaseChecklist); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(riskSummaryJSON, &m.RiskSummary); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(rejectionJSON, &m.LastReleaseRejection); err != nil {
		return nil, err
	}
	if len(planHistoryJSON) > 0 {
		_ = json.Unmarshal(planHistoryJSON, &m.PlanHistory)
	}
	if len(verificationHistoryJSON) > 0 {
		_ = json.Unmarshal(verificationHistoryJSON, &m.VerificationHistory)
	}
	if len(qualificationsJSON) > 0 {
		_ = json.Unmarshal(qualificationsJSON, &m.MemberQualifications)
	}
	if len(riskHistoryJSON) > 0 {
		_ = json.Unmarshal(riskHistoryJSON, &m.RiskHistory)
	}
	m.ScheduleCheck = mission.ScheduleCheck{Checked: true, CaveSiteKey: m.CaveSiteKey, Conflicts: []mission.ScheduleConflict{}}
	if err := loadSegmentsAndMembers(ctx, q, m); err != nil {
		return nil, err
	}
	if len(m.MemberQualifications) > 0 {
		_, m.QualificationStatus, _ = mission.ValidateQualifications(m.TeamMembers, m.MemberQualifications, m.TargetDepthM, m.WindowEnd)
	}
	if err := loadRisks(ctx, q, m); err != nil {
		return nil, err
	}
	if err := loadPlan(ctx, q, m); err != nil {
		return nil, err
	}
	if err := loadVerifications(ctx, q, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (t *transaction) InsertMission(ctx context.Context, m *mission.DiveMission) error {
	checklist, _ := json.Marshal(m.ReleaseChecklist)
	riskSummary, _ := json.Marshal(m.RiskSummary)
	rejection, _ := json.Marshal(m.LastReleaseRejection)
	planHistory, _ := json.Marshal(m.PlanHistory)
	verificationHistory, _ := json.Marshal(m.VerificationHistory)
	qualifications, _ := json.Marshal(m.MemberQualifications)
	riskHistory, _ := json.Marshal(m.RiskHistory)
	_, err := t.tx.ExecContext(ctx, `INSERT INTO missions(id,title,cave_site,cave_site_key,target_depth_m,window_start,window_end,status,revision,leader_id,release_digest,release_checklist,signed_by,archive_digest,risk_summary,last_release_rejection,created_at,updated_at,archived_at,plan_history,verification_history,template_mission_id,template_archive_digest,qualification_snapshot,risk_history) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.Title, m.CaveSite, m.CaveSiteKey, m.TargetDepthM, stamp(m.WindowStart), stamp(m.WindowEnd), m.Status, m.Revision, m.LeaderID, m.ReleaseDigest, checklist, m.SignedBy, m.ArchiveDigest, riskSummary, rejection, stamp(m.CreatedAt), stamp(m.UpdatedAt), nullableTime(m.ArchivedAt), planHistory, verificationHistory, m.TemplateMissionID, m.TemplateArchiveDigest, qualifications, riskHistory)
	if err != nil {
		return err
	}
	return saveSegmentsAndMembers(ctx, t.tx, m)
}

func (t *transaction) SaveMission(ctx context.Context, m *mission.DiveMission, expected int64) error {
	checklist, _ := json.Marshal(m.ReleaseChecklist)
	riskSummary, _ := json.Marshal(m.RiskSummary)
	rejection, _ := json.Marshal(m.LastReleaseRejection)
	planHistory, _ := json.Marshal(m.PlanHistory)
	verificationHistory, _ := json.Marshal(m.VerificationHistory)
	qualifications, _ := json.Marshal(m.MemberQualifications)
	riskHistory, _ := json.Marshal(m.RiskHistory)
	result, err := t.tx.ExecContext(ctx, `UPDATE missions SET title=?,cave_site=?,cave_site_key=?,target_depth_m=?,window_start=?,window_end=?,status=?,revision=?,leader_id=?,release_digest=?,release_checklist=?,signed_by=?,archive_digest=?,risk_summary=?,last_release_rejection=?,updated_at=?,archived_at=?,plan_history=?,verification_history=?,template_mission_id=?,template_archive_digest=?,qualification_snapshot=?,risk_history=? WHERE id=? AND revision=?`,
		m.Title, m.CaveSite, m.CaveSiteKey, m.TargetDepthM, stamp(m.WindowStart), stamp(m.WindowEnd), m.Status, m.Revision, m.LeaderID, m.ReleaseDigest, checklist, m.SignedBy, m.ArchiveDigest, riskSummary, rejection, stamp(m.UpdatedAt), nullableTime(m.ArchivedAt), planHistory, verificationHistory, m.TemplateMissionID, m.TemplateArchiveDigest, qualifications, riskHistory, m.ID, expected)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return mission.ErrConflict
	}
	if _, err := t.tx.ExecContext(ctx, `DELETE FROM mission_segments WHERE mission_id=?`, m.ID); err != nil {
		return err
	}
	if _, err := t.tx.ExecContext(ctx, `DELETE FROM mission_members WHERE mission_id=?`, m.ID); err != nil {
		return err
	}
	if err := saveSegmentsAndMembers(ctx, t.tx, m); err != nil {
		return err
	}
	if err := replaceRisks(ctx, t.tx, m); err != nil {
		return err
	}
	if err := replacePlan(ctx, t.tx, m); err != nil {
		return err
	}
	return replaceVerifications(ctx, t.tx, m)
}

func (t *transaction) ScheduleConflicts(ctx context.Context, siteKey string, start, end time.Time) ([]mission.ScheduleConflict, error) {
	return t.ScheduleConflictsExcluding(ctx, siteKey, start, end, "")
}
func (t *transaction) ScheduleConflictsExcluding(ctx context.Context, siteKey string, start, end time.Time, exclude string) ([]mission.ScheduleConflict, error) {
	rows, err := t.tx.QueryContext(ctx, `SELECT id,window_start,window_end,status FROM missions WHERE cave_site_key=? AND status<>? AND id<>? AND window_start<? AND window_end>? ORDER BY window_start,id`, siteKey, mission.StatusArchived, exclude, stamp(end), stamp(start))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var conflicts []mission.ScheduleConflict
	for rows.Next() {
		var c mission.ScheduleConflict
		var from, to string
		if err := rows.Scan(&c.MissionID, &from, &to, &c.Status); err != nil {
			return nil, err
		}
		c.WindowStart, err = time.Parse(time.RFC3339Nano, from)
		if err != nil {
			return nil, err
		}
		c.WindowEnd, err = time.Parse(time.RFC3339Nano, to)
		if err != nil {
			return nil, err
		}
		conflicts = append(conflicts, c)
	}
	return conflicts, rows.Err()
}

func (s *Store) SchedulePreflight(ctx context.Context, siteKey string, start, end time.Time) ([]mission.ScheduleConflict, error) {
	lock := s.missionLock("__mission_transactions__")
	lock.Lock()
	defer lock.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	conflicts, err := (&transaction{tx: tx}).ScheduleConflicts(ctx, siteKey, start, end)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return conflicts, nil
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func nullableTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return stamp(*t)
}
