package mission

import "context"

type ReleasePreview struct {
	MissionID      string              `json:"mission_id"`
	SourceRevision int64               `json:"source_revision"`
	PreviewDigest  string              `json:"preview_digest"`
	Checklist      []ChecklistItem     `json:"checklist"`
	Blockers       []ChecklistItem     `json:"blockers"`
	SupervisorID   string              `json:"supervisor_id,omitempty"`
	Qualification  []string            `json:"qualification,omitempty"`
	Passed         bool                `json:"passed"`
	LastRejection  *ReleaseRejection   `json:"last_rejection,omitempty"`
	SourceRecords  []EvidenceReference `json:"source_records"`
	Lineage        []EvidenceReference `json:"lineage"`
}

func (s *Service) PreviewRelease(ctx context.Context, id, supervisor string) (ReleasePreview, error) {
	m, err := s.Mission(ctx, id)
	if err != nil {
		return ReleasePreview{}, err
	}
	events, err := s.repo.AllEvents(ctx, id)
	if err != nil {
		return ReleasePreview{}, err
	}
	c, err := traceReleaseChecklist(m, events)
	if err != nil {
		return ReleasePreview{}, err
	}
	blockers := []ChecklistItem{}
	for _, i := range c {
		if !i.Passed {
			blockers = append(blockers, i)
		}
	}
	qualification := supervisorConflicts(m, supervisor)
	digest, err := commandFingerprint("release_preview", CommandMeta{ExpectedRevision: m.Revision}, struct {
		Checklist []ChecklistItem `json:"checklist"`
	}{c})
	if err != nil {
		return ReleasePreview{}, err
	}
	sources, lineage := []EvidenceReference{}, []EvidenceReference{}
	for _, item := range c {
		sources = append(sources, item.SourceRecords...)
		lineage = append(lineage, item.Lineage...)
	}
	return ReleasePreview{MissionID: m.ID, SourceRevision: m.Revision, PreviewDigest: digest, Checklist: c, Blockers: blockers, SupervisorID: supervisor, Qualification: qualification, Passed: len(blockers) == 0 && len(qualification) == 0, LastRejection: m.LastReleaseRejection, SourceRecords: sources, Lineage: lineage}, nil
}

func supervisorConflicts(m *DiveMission, actor string) []string {
	if actor == "" {
		return nil
	}
	out := []string{}
	if memberIDs(m)[actor] {
		out = append(out, "supervisor_is_mission_member")
	}
	if m.LifeSupportPlan != nil && m.LifeSupportPlan.ReviewedBy == actor {
		out = append(out, "supervisor_is_plan_reviewer")
	}
	for _, r := range append(append([]VerificationRecord{}, m.Verifications...), m.VerificationHistory...) {
		if r.VerifiedBy != actor {
			continue
		}
		switch r.RecordType {
		case "equipment":
			out = append(out, "supervisor_is_equipment_verifier")
		case "drill":
			out = append(out, "supervisor_is_drill_witness")
		case "remediation":
			out = append(out, "supervisor_is_remediation_actor")
		case "retest":
			out = append(out, "supervisor_is_retest_actor")
		}
	}
	seen := map[string]bool{}
	dedup := out[:0]
	for _, v := range out {
		if !seen[v] {
			seen[v] = true
			dedup = append(dedup, v)
		}
	}
	return dedup
}
