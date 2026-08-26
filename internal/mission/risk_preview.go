package mission

import (
	"context"
	"sort"
	"strings"

	"benzhi-project-cdee2d20-0f7a-4d82-85d2-5bf67fbdd030/internal/audit"
)

const riskRuleVersion = "cave-risk-v1"

type RiskIssue struct {
	SegmentName  string `json:"segment_name"`
	Field        string `json:"field"`
	Code         string `json:"code"`
	Message      string `json:"message"`
	MissingCount int    `json:"missing_count,omitempty"`
}

type RiskPreview struct {
	MissionID         string        `json:"mission_id"`
	SourceRevision    int64         `json:"source_revision"`
	RuleVersion       string        `json:"rule_version"`
	RiskPreviewDigest string        `json:"risk_preview_digest"`
	Risks             []SegmentRisk `json:"risks"`
	Summary           RiskSummary   `json:"summary"`
	Issues            []RiskIssue   `json:"issues"`
	Gaps              []RiskIssue   `json:"gaps"`
	Passed            bool          `json:"passed"`
}

func scoreRisk(risk SegmentRisk) SegmentRisk {
	breakdown := RiskScoreBreakdown{CurrentLevel: risk.CurrentLevel, RestrictionGrade: risk.RestrictionGrade}
	if risk.VisibilityM < 3 {
		breakdown.Visibility = 2
	} else if risk.VisibilityM < 6 {
		breakdown.Visibility = 1
	}
	if risk.ExitLimitMin > 90 {
		breakdown.ExitLimit = 2
	} else if risk.ExitLimitMin > 60 {
		breakdown.ExitLimit = 1
	}
	risk.ScoreBreakdown = breakdown
	risk.Score = breakdown.CurrentLevel + breakdown.RestrictionGrade + breakdown.Visibility + breakdown.ExitLimit
	switch {
	case risk.Score >= 10:
		risk.RiskLevel, risk.RiskExplanation = "critical", "极高风险：环境与撤离指标叠加，必须采用至少两项独立缓解措施"
	case risk.Score >= 7:
		risk.RiskLevel, risk.RiskExplanation = "high", "高风险：至少两项指标显著受限，必须采用至少一项缓解措施"
	case risk.Score >= 4:
		risk.RiskLevel, risk.RiskExplanation = "medium", "中风险：存在可控限制，须维持已登记缓解措施"
	default:
		risk.RiskLevel, risk.RiskExplanation = "low", "低风险：当前指标处于常规控制范围"
	}
	return risk
}

func calculateRiskPreview(m *DiveMission, risks []SegmentRisk) (RiskPreview, error) {
	preview := RiskPreview{MissionID: m.ID, SourceRevision: m.Revision, RuleVersion: riskRuleVersion}
	wanted, seen := map[string]bool{}, map[string]bool{}
	for _, name := range m.Segments {
		wanted[name] = true
	}
	for _, raw := range risks {
		r := raw
		r.SegmentName = strings.TrimSpace(r.SegmentName)
		if r.SegmentName == "" || !wanted[r.SegmentName] {
			preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "segment_name", "segment_unknown", "洞段必须与任务草案一致", 0})
		} else if seen[r.SegmentName] {
			preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "segment_name", "segment_duplicate", "洞段不能重复", 0})
		}
		seen[r.SegmentName] = true
		if r.CurrentLevel < 1 || r.CurrentLevel > 5 {
			preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "current_level", "range_invalid", "必须在 1 到 5 之间", 0})
		}
		if r.VisibilityM <= 0 || r.VisibilityM > 100 {
			preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "visibility_m", "range_invalid", "必须在 0 到 100 米之间", 0})
		}
		if r.RestrictionGrade < 1 || r.RestrictionGrade > 5 {
			preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "restriction_grade", "range_invalid", "必须在 1 到 5 之间", 0})
		}
		if r.ExitLimitMin <= 0 || r.ExitLimitMin > 240 {
			preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "exit_limit_min", "range_invalid", "必须在 1 到 240 分钟之间", 0})
		}
		if len(r.Hazards) == 0 {
			preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "hazards", "required", "至少登记一项危险", 0})
		}
		for _, value := range r.Hazards {
			if strings.TrimSpace(value) == "" {
				preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "hazards", "empty_item", "危险项不能是空内容", 0})
				break
			}
		}
		for _, value := range r.Mitigations {
			if strings.TrimSpace(value) == "" {
				preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "mitigations", "empty_item", "缓解措施不能是空内容", 0})
				break
			}
		}
		r = scoreRisk(r)
		required := map[string]int{"critical": 2, "high": 1}[r.RiskLevel]
		if len(r.Mitigations) < required {
			preview.Issues = append(preview.Issues, RiskIssue{r.SegmentName, "mitigations", "mitigation_gap", "高危洞段缓解措施不足", required - len(r.Mitigations)})
		}
		preview.Risks = append(preview.Risks, r)
	}
	for _, name := range m.Segments {
		if !seen[name] {
			preview.Issues = append(preview.Issues, RiskIssue{name, "segment_name", "segment_missing", "缺少任务洞段风险", 0})
		}
	}
	sort.Slice(preview.Risks, func(i, j int) bool { return preview.Risks[i].SegmentName < preview.Risks[j].SegmentName })
	sort.Slice(preview.Issues, func(i, j int) bool {
		if preview.Issues[i].SegmentName == preview.Issues[j].SegmentName {
			if preview.Issues[i].Field == preview.Issues[j].Field {
				return preview.Issues[i].Code < preview.Issues[j].Code
			}
			return preview.Issues[i].Field < preview.Issues[j].Field
		}
		return preview.Issues[i].SegmentName < preview.Issues[j].SegmentName
	})
	preview.Summary = SummarizeRisks(preview.Risks)
	preview.Gaps = append([]RiskIssue(nil), preview.Issues...)
	canonical := append([]SegmentRisk(nil), preview.Risks...)
	for i := range canonical {
		canonical[i].ID, canonical[i].MissionID, canonical[i].AssessedBy = "", "", ""
		canonical[i].Hazards = append([]string(nil), canonical[i].Hazards...)
		sort.Strings(canonical[i].Hazards)
		canonical[i].Mitigations = append([]string(nil), canonical[i].Mitigations...)
		sort.Strings(canonical[i].Mitigations)
	}
	digest, err := audit.Digest(struct {
		Revision int64         `json:"source_revision"`
		Rule     string        `json:"rule_version"`
		Risks    []SegmentRisk `json:"risks"`
	}{m.Revision, riskRuleVersion, canonical})
	preview.RiskPreviewDigest, preview.Passed = digest, len(preview.Issues) == 0
	return preview, err
}

func (s *Service) PreviewRisks(ctx context.Context, id string, risks []SegmentRisk) (RiskPreview, error) {
	m, err := s.repo.Mission(ctx, id)
	if err != nil {
		return RiskPreview{}, err
	}
	if m.Status != StatusDraft {
		return RiskPreview{}, InvalidState(m.Status, "风险评估试算")
	}
	return calculateRiskPreview(m, risks)
}
