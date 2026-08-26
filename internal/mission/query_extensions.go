package mission

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

type TemplatePreviewInput struct {
	RequestID         string
	TemplateMissionID string
	Title             string
	TeamMembers       []Member
	WindowStart       time.Time
	WindowEnd         time.Time
}

func (s *Service) verifyQueryAudit(ctx context.Context, m *DiveMission) error {
	events, err := s.repo.AllEvents(ctx, m.ID)
	if err != nil || audit.Verify(events) != nil {
		return NewError("audit_integrity_failed", "审计链完整性校验失败", 409)
	}
	if m.Status == StatusArchived {
		if len(events) < 2 || events[len(events)-1].EventType != "mission_archived" || events[len(events)-2].EventHash != m.ArchiveDigest {
			return NewError("archive_integrity_failed", "归档链完整性校验失败", 409)
		}
	}
	return nil
}

type TemplatePreview struct {
	RequestID             string                `json:"request_id"`
	ReadOnly              bool                  `json:"read_only"`
	TemplateMissionID     string                `json:"template_mission_id"`
	TemplateRevision      int64                 `json:"template_revision"`
	TemplateArchiveDigest string                `json:"template_archive_digest"`
	Title                 string                `json:"title"`
	CaveSite              string                `json:"cave_site"`
	CaveSiteKey           string                `json:"cave_site_key"`
	TargetDepthM          float64               `json:"target_depth_m"`
	Segments              []string              `json:"segments"`
	TeamMembers           []Member              `json:"team_members"`
	MemberQualifications  []MemberQualification `json:"member_qualifications"`
	QualificationStatus   []QualificationStatus `json:"qualification_status"`
	Conflicts             []ScheduleConflict    `json:"conflicts"`
	Available             bool                  `json:"available"`
	PreviewDigest         string                `json:"preview_digest"`
	Conditions            []string              `json:"conditions,omitempty"`
}

func (s *Service) PreviewTemplate(ctx context.Context, in TemplatePreviewInput) (TemplatePreview, error) {
	if strings.TrimSpace(in.RequestID) == "" || strings.TrimSpace(in.TemplateMissionID) == "" {
		return TemplatePreview{}, Invalid("request_id", "request_id 和模板标识不能为空")
	}
	if in.WindowStart.IsZero() || in.WindowEnd.IsZero() || !in.WindowEnd.After(in.WindowStart) {
		return TemplatePreview{}, Invalid("window", "时间窗无效")
	}
	m, err := s.repo.Mission(ctx, strings.TrimSpace(in.TemplateMissionID))
	if err != nil {
		return TemplatePreview{}, err
	}
	if m.Status != StatusArchived {
		return TemplatePreview{}, NewError("template_not_archived", "模板任务必须已经归档", 422)
	}
	events, err := s.repo.AllEvents(ctx, m.ID)
	if err != nil || audit.Verify(events) != nil || len(events) < 2 || events[len(events)-1].EventType != "mission_archived" || events[len(events)-2].EventHash != m.ArchiveDigest {
		return TemplatePreview{}, NewError("template_integrity_failed", "模板归档审计链或归档摘要校验失败", 409)
	}
	members := append([]Member(nil), m.TeamMembers...)
	if len(in.TeamMembers) > 0 {
		known := map[string]bool{}
		for _, x := range members {
			known[x.PersonID] = true
		}
		seen := map[string]bool{}
		for _, x := range in.TeamMembers {
			if !known[x.PersonID] || seen[x.PersonID] {
				return TemplatePreview{}, Unprocessable("team_members", "覆盖成员必须来自模板且职责唯一", map[string]any{"person_id": x.PersonID})
			}
			seen[x.PersonID] = true
		}
		for i := range members {
			for _, x := range in.TeamMembers {
				if x.PersonID == members[i].PersonID && strings.TrimSpace(x.Name) != "" {
					members[i].Name = strings.TrimSpace(x.Name)
				}
			}
		}
	}
	var quals []MemberQualification
	var statuses []QualificationStatus
	if len(m.MemberQualifications) > 0 {
		quals, statuses, err = ValidateQualifications(members, m.MemberQualifications, m.TargetDepthM, in.WindowEnd.UTC())
		if err != nil {
			return TemplatePreview{}, err
		}
	}
	conflicts, err := s.repo.SchedulePreflight(ctx, m.CaveSiteKey, in.WindowStart.UTC(), in.WindowEnd.UTC())
	if err != nil {
		return TemplatePreview{}, err
	}
	conditions := []string{}
	if len(conflicts) > 0 {
		conditions = append(conditions, "schedule_conflict")
	}
	for _, q := range statuses {
		if !q.Passed {
			conditions = append(conditions, "qualification_failed")
			break
		}
	}
	result := TemplatePreview{RequestID: in.RequestID, ReadOnly: true, TemplateMissionID: m.ID, TemplateRevision: m.Revision, TemplateArchiveDigest: m.ArchiveDigest, Title: strings.TrimSpace(in.Title), CaveSite: m.CaveSite, CaveSiteKey: m.CaveSiteKey, TargetDepthM: m.TargetDepthM, Segments: append([]string(nil), m.Segments...), TeamMembers: members, MemberQualifications: quals, QualificationStatus: statuses, Conflicts: conflicts, Available: len(conflicts) == 0 && len(conditions) == 0, Conditions: conditions}
	if result.Title == "" {
		result.Title = m.Title
	}
	digest, _ := audit.Digest(struct {
		Template string   `json:"template"`
		Revision int64    `json:"revision"`
		Title    string   `json:"title"`
		Members  []Member `json:"members"`
		Start    string   `json:"start"`
		End      string   `json:"end"`
	}{m.ID, m.Revision, result.Title, members, in.WindowStart.UTC().Format(time.RFC3339Nano), in.WindowEnd.UTC().Format(time.RFC3339Nano)})
	result.PreviewDigest = digest
	return result, nil
}

type MitigationQueryFilter struct {
	Segment, Owner, Status string
	From, To               *time.Time
	Cursor                 string
	Limit                  int
}
type MitigationItem struct {
	SegmentName string           `json:"segment_name"`
	Action      MitigationAction `json:"action"`
	Status      string           `json:"status"`
	Overdue     bool             `json:"overdue"`
	Blocker     string           `json:"blocker,omitempty"`
}
type MitigationQueryResult struct {
	Items           []MitigationItem `json:"items"`
	Statistics      map[string]int   `json:"statistics"`
	EarliestOverdue *time.Time       `json:"earliest_overdue,omitempty"`
	NextCursor      string           `json:"next_cursor,omitempty"`
	SourceRevision  int64            `json:"source_revision"`
}

func (s *Service) QueryMitigations(ctx context.Context, id string, f MitigationQueryFilter) (MitigationQueryResult, error) {
	m, err := s.Mission(ctx, id)
	if err != nil {
		return MitigationQueryResult{}, err
	}
	if err := s.verifyQueryAudit(ctx, m); err != nil {
		return MitigationQueryResult{}, err
	}
	now := s.now().UTC()
	var all []MitigationItem
	riskSets := append(append([]SegmentRisk(nil), m.RiskHistory...), m.Risks...)
	for _, r := range riskSets {
		for _, a := range r.MitigationActions {
			status := a.Status
			overdue := status != "completed" && !a.DueAt.IsZero() && now.After(a.DueAt)
			if overdue {
				status = "overdue"
			}
			if f.Segment != "" && f.Segment != r.SegmentName || f.Owner != "" && f.Owner != a.OwnerPersonID || f.Status != "" && f.Status != status || f.From != nil && a.DueAt.Before(*f.From) || f.To != nil && a.DueAt.After(*f.To) {
				continue
			}
			blocker := ""
			if status != "completed" {
				blocker = "risk_mitigation_incomplete"
			}
			all = append(all, MitigationItem{r.SegmentName, a, status, overdue, blocker})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].SegmentName == all[j].SegmentName {
			return all[i].Action.Code < all[j].Action.Code
		}
		return all[i].SegmentName < all[j].SegmentName
	})
	stats := map[string]int{"open": 0, "completed": 0, "overdue": 0}
	for _, x := range all {
		stats[x.Status]++
	}
	start := 0
	if f.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		if e != nil {
			return MitigationQueryResult{}, Invalid("cursor", "游标格式无效")
		}
		var c struct{ Segment, Code string }
		if json.Unmarshal(b, &c) != nil {
			return MitigationQueryResult{}, Invalid("cursor", "游标格式无效")
		}
		found := false
		for i, x := range all {
			if x.SegmentName == c.Segment && x.Action.Code == c.Code {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return MitigationQueryResult{}, Invalid("cursor", "游标与筛选条件不匹配")
		}
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	end := start + f.Limit
	if end > len(all) {
		end = len(all)
	}
	out := MitigationQueryResult{Items: all[start:end], Statistics: stats, SourceRevision: m.Revision}
	for _, x := range all {
		if x.Overdue && (out.EarliestOverdue == nil || x.Action.DueAt.Before(*out.EarliestOverdue)) {
			t := x.Action.DueAt
			out.EarliestOverdue = &t
		}
	}
	if end < len(all) {
		b, _ := json.Marshal(struct{ Segment, Code string }{all[end-1].SegmentName, all[end-1].Action.Code})
		out.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	return out, nil
}

type EquipmentEvidenceFilter struct {
	Status, CheckCode, AssetID, Cursor string
	Limit                              int
}
type EquipmentEvidenceItem struct {
	CheckCode        string               `json:"check_code"`
	Label            string               `json:"label"`
	Status           string               `json:"status"`
	Current          *VerificationRecord  `json:"current,omitempty"`
	ReplacementChain []VerificationRecord `json:"replacement_chain,omitempty"`
	BlocksDrill      bool                 `json:"blocks_drill"`
	SourceRevision   int64                `json:"source_revision"`
}
type EquipmentEvidenceResult struct {
	Items          []EquipmentEvidenceItem `json:"items"`
	NextCursor     string                  `json:"next_cursor,omitempty"`
	SourceRevision int64                   `json:"source_revision"`
}

func (s *Service) QueryEquipmentEvidence(ctx context.Context, id string, f EquipmentEvidenceFilter) (EquipmentEvidenceResult, error) {
	m, err := s.Mission(ctx, id)
	if err != nil {
		return EquipmentEvidenceResult{}, err
	}
	if err := s.verifyQueryAudit(ctx, m); err != nil {
		return EquipmentEvidenceResult{}, err
	}
	if f.CheckCode != "" {
		if _, ok := equipmentLabels[f.CheckCode]; !ok {
			return EquipmentEvidenceResult{}, Invalid("check_code", "未知装备检查代码")
		}
	}
	all := append(append([]VerificationRecord(nil), m.VerificationHistory...), m.Verifications...)
	latest := map[string]VerificationRecord{}
	for _, r := range all {
		if r.RecordType == "equipment" {
			latest[r.CheckCode] = r
		}
	}
	now := s.now().UTC()
	var items []EquipmentEvidenceItem
	codes := sortedEquipmentCodes()
	for _, code := range codes {
		r, ok := latest[code]
		var status string
		if !ok {
			status = "missing"
		} else if r.ValidUntil.Before(now) {
			status = "expired"
		} else if r.ValidUntil.Before(now.Add(72 * time.Hour)) {
			status = "expiring"
		} else {
			status = "valid"
		}
		var ptr *VerificationRecord
		if ok {
			rr := r
			ptr = &rr
		}
		if f.CheckCode != "" && f.CheckCode != code || f.AssetID != "" && (!ok || r.AssetID != f.AssetID) || f.Status != "" && f.Status != status {
			continue
		}
		chain := []VerificationRecord{}
		if ok {
			chain = append(chain, r)
			want := r.ReplacementForAssetID
			for want != "" {
				found := false
				for _, x := range all {
					if x.RecordType == "equipment" && x.AssetID == want {
						chain = append(chain, x)
						want = x.ReplacementForAssetID
						found = true
						break
					}
				}
				if !found {
					break
				}
			}
			sort.Slice(chain, func(i, j int) bool { return chain[i].RecordedAt.Before(chain[j].RecordedAt) })
		}
		items = append(items, EquipmentEvidenceItem{code, equipmentLabels[code], status, ptr, chain, status != "valid" || !ok, m.Revision})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CheckCode < items[j].CheckCode })
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	start := 0
	if f.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		if e != nil {
			return EquipmentEvidenceResult{}, Invalid("cursor", "游标格式无效")
		}
		var c string
		if json.Unmarshal(b, &c) != nil {
			return EquipmentEvidenceResult{}, Invalid("cursor", "游标格式无效")
		}
		for i, x := range items {
			if x.CheckCode == c {
				start = i + 1
			}
		}
		if start == 0 {
			return EquipmentEvidenceResult{}, Invalid("cursor", "游标与筛选条件不匹配")
		}
	}
	end := start + f.Limit
	if end > len(items) {
		end = len(items)
	}
	res := EquipmentEvidenceResult{Items: items[start:end], SourceRevision: m.Revision}
	if end < len(items) {
		b, _ := json.Marshal(items[end-1].CheckCode)
		res.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	return res, nil
}

type RemediationReviewFilter struct {
	CheckCode string
	Cycle     int
	Status    string
	Overdue   *bool
	Cursor    string
	Limit     int
}
type RemediationReviewResult struct {
	Items          []map[string]any `json:"items"`
	Statistics     map[string]int   `json:"statistics"`
	SourceRevision int64            `json:"source_revision"`
	NextCursor     string           `json:"next_cursor,omitempty"`
}

// 兼容业务层常用命名，均保持只读语义。
func (s *Service) DeriveTemplatePreview(ctx context.Context, in TemplatePreviewInput) (TemplatePreview, error) {
	return s.PreviewTemplate(ctx, in)
}
func (s *Service) RiskMitigations(ctx context.Context, id string, f MitigationQueryFilter) (MitigationQueryResult, error) {
	return s.QueryMitigations(ctx, id, f)
}
func (s *Service) EquipmentEvidence(ctx context.Context, id string, f EquipmentEvidenceFilter) (EquipmentEvidenceResult, error) {
	return s.QueryEquipmentEvidence(ctx, id, f)
}
func (s *Service) RemediationReview(ctx context.Context, id string, f RemediationReviewFilter) (RemediationReviewResult, error) {
	return s.QueryRemediationReview(ctx, id, f)
}

func (s *Service) QueryRemediationReview(ctx context.Context, id string, f RemediationReviewFilter) (RemediationReviewResult, error) {
	m, err := s.Mission(ctx, id)
	if err != nil {
		return RemediationReviewResult{}, err
	}
	if err := s.verifyQueryAudit(ctx, m); err != nil {
		return RemediationReviewResult{}, err
	}
	var out []map[string]any
	now := s.now().UTC()
	for _, code := range []string{"lost_contact", "gas_sharing"} {
		for _, rem := range verificationRecordsFor(m, "remediation", code) {
			status := "awaiting_retest"
			var ret *VerificationRecord
			for _, x := range verificationRecordsFor(m, "retest", code) {
				if x.RemediationCycle == rem.RemediationCycle {
					y := x
					ret = &y
					if x.Outcome == "pass" {
						status = "passed"
					} else {
						status = "failed"
					}
					break
				}
			}
			overdue := rem.WasOverdue || (!rem.RemediationDueAt.IsZero() && now.After(rem.RemediationDueAt) && ret == nil)
			if overdue && status == "awaiting_retest" {
				status = "overdue"
			}
			if f.CheckCode != "" && f.CheckCode != code || f.Cycle > 0 && f.Cycle != rem.RemediationCycle || f.Status != "" && f.Status != status || f.Overdue != nil && *f.Overdue != overdue {
				continue
			}
			duration := int64(0)
			if !rem.RecordedAt.IsZero() {
				end := rem.RecordedAt
				if ret != nil {
					end = ret.RecordedAt
				}
				duration = int64(end.Sub(rem.RecordedAt).Seconds())
			}
			out = append(out, map[string]any{"check_code": code, "cycle": rem.RemediationCycle, "status": status, "overdue": overdue, "remediation": rem, "retest": ret, "duration_seconds": duration, "delay_seconds": rem.DelaySeconds, "blocking": status != "passed"})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a["check_code"] == b["check_code"] {
			return a["cycle"].(int) < b["cycle"].(int)
		}
		return a["check_code"].(string) < b["check_code"].(string)
	})
	stats := map[string]int{"awaiting_retest": 0, "failed": 0, "passed": 0, "overdue": 0}
	for _, x := range out {
		stats[x["status"].(string)]++
	}
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	start := 0
	if f.Cursor != "" {
		b, e := base64.RawURLEncoding.DecodeString(f.Cursor)
		if e != nil {
			return RemediationReviewResult{}, Invalid("cursor", "游标格式无效")
		}
		var c struct {
			Code  string
			Cycle int
		}
		if json.Unmarshal(b, &c) != nil {
			return RemediationReviewResult{}, Invalid("cursor", "游标格式无效")
		}
		for i, x := range out {
			if x["check_code"] == c.Code && x["cycle"] == c.Cycle {
				start = i + 1
			}
		}
		if start == 0 {
			return RemediationReviewResult{}, Invalid("cursor", "游标与筛选条件不匹配")
		}
	}
	end := start + f.Limit
	if end > len(out) {
		end = len(out)
	}
	res := RemediationReviewResult{Items: out[start:end], Statistics: stats, SourceRevision: m.Revision}
	if end < len(out) {
		b, _ := json.Marshal(struct {
			Code  string
			Cycle int
		}{out[end-1]["check_code"].(string), out[end-1]["cycle"].(int)})
		res.NextCursor = base64.RawURLEncoding.EncodeToString(b)
	}
	return res, nil
}
