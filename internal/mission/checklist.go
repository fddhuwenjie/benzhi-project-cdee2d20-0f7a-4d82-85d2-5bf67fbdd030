package mission

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

var equipmentLabels = map[string]string{
	"primary_breathing": "主呼吸系统",
	"backup_breathing":  "备用呼吸系统",
	"primary_lighting":  "主备照明",
	"guideline":         "导向绳系统",
	"communication":     "通信装备",
}

var drillLabels = map[string]string{
	"lost_contact": "失联演练",
	"gas_sharing":  "气体共享演练",
}

func latestRecords(records []VerificationRecord, recordType string) map[string]VerificationRecord {
	result := map[string]VerificationRecord{}
	for _, record := range records {
		if record.RecordType == recordType {
			result[record.CheckCode] = record
		}
	}
	return result
}

func EquipmentComplete(m *DiveMission) bool {
	return EquipmentCompleteAt(m, time.Now().UTC())
}
func EquipmentCompleteAt(m *DiveMission, now time.Time) bool {
	records := latestRecords(m.Verifications, "equipment")
	for code := range equipmentLabels {
		r := records[code]
		if r.Outcome != "pass" || r.AssetID == "" || r.ValidUntil.Before(m.WindowEnd) || r.ValidUntil.Before(now) {
			return false
		}
	}
	return true
}

func DrillsComplete(m *DiveMission) bool {
	records := latestRecords(m.Verifications, "drill")
	remediations := latestRecords(m.Verifications, "remediation")
	retests := latestRecords(m.Verifications, "retest")
	for code := range drillLabels {
		if records[code].Outcome == "pass" {
			continue
		}
		if records[code].Outcome != "deviation" || remediations[code].CorrectiveAction == "" || retests[code].Outcome != "pass" {
			return false
		}
	}
	return true
}

func unresolvedDeviations(m *DiveMission) []string {
	drills := latestRecords(m.Verifications, "drill")
	remediations := latestRecords(m.Verifications, "remediation")
	retests := latestRecords(m.Verifications, "retest")
	var result []string
	for code := range drillLabels {
		if drills[code].Outcome == "deviation" && (remediations[code].CorrectiveAction == "" || retests[code].Outcome != "pass") {
			result = append(result, code)
		}
	}
	sort.Strings(result)
	return result
}

func BuildReleaseChecklist(m *DiveMission) []ChecklistItem {
	riskPassed := len(m.Risks) == len(m.Segments) && m.Status != StatusDraft
	planPassed := m.LifeSupportPlan != nil && m.LifeSupportPlan.ReviewStatus == "approved" && m.LifeSupportPlan.CrossCheck.Passed
	equipmentPassed := EquipmentComplete(m)
	drillsPassed := DrillsComplete(m) && len(unresolvedDeviations(m)) == 0
	items := []ChecklistItem{
		{Code: "risk_assessment", Label: "全部洞段风险已评估", Passed: riskPassed, Detail: "风险记录完整", CurrentValue: fmt.Sprintf("%d/%d", len(m.Risks), len(m.Segments)), RequiredValue: fmt.Sprintf("%d/%d", len(m.Segments), len(m.Segments))},
		{Code: "life_support_plan", Label: "生命支持方案已独立审核", Passed: planPassed, Detail: "人员、气体与转向压力方案", CurrentValue: planCurrentValue(m), RequiredValue: "approved_and_cross_checked"},
		{Code: "equipment", Label: "必检装备证据完整", Passed: equipmentPassed, Detail: "主备呼吸、照明、导向绳和通信", CurrentValue: strings.Join(equipmentCodes(m, true), ","), RequiredValue: strings.Join(sortedEquipmentCodes(), ",")},
		{Code: "drills", Label: "应急演练与复验通过", Passed: drillsPassed, Detail: "失联与气体共享演练", CurrentValue: drillCurrentValue(m), RequiredValue: "lost_contact,gas_sharing 均通过或完成整改复验"},
	}
	for i := range items {
		if !items[i].Passed {
			items[i].MissingReason = items[i].Label + "未满足"
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Code < items[j].Code })
	return items
}

func sortedEquipmentCodes() []string {
	result := make([]string, 0, len(equipmentLabels))
	for code := range equipmentLabels {
		result = append(result, code)
	}
	sort.Strings(result)
	return result
}
func planCurrentValue(m *DiveMission) string {
	if m.LifeSupportPlan == nil {
		return "missing"
	}
	return m.LifeSupportPlan.ReviewStatus + fmt.Sprintf(";cross_check=%t", m.LifeSupportPlan.CrossCheck.Passed)
}
func drillCurrentValue(m *DiveMission) string {
	records := latestRecords(m.Verifications, "drill")
	var values []string
	for _, code := range []string{"gas_sharing", "lost_contact"} {
		values = append(values, code+"="+records[code].Outcome)
	}
	for _, record := range latestRecords(m.Verifications, "remediation") {
		if record.WasOverdue {
			values = append(values, fmt.Sprintf("%s_overdue=%ds", record.CheckCode, record.DelaySeconds))
		}
	}
	sort.Strings(values)
	return strings.Join(values, ",")
}

func checklistPassed(items []ChecklistItem) bool {
	for _, item := range items {
		if !item.Passed {
			return false
		}
	}
	return true
}

func AllowedActions(m *DiveMission) []string {
	var actions []string
	switch m.Status {
	case StatusDraft:
		actions = []string{"revise_draft", "submit_risks"}
	case StatusRiskAssessed:
		actions = []string{"reassess_risks", "submit_life_support_plan"}
		if len(mitigationBlockers(m, time.Now().UTC())) > 0 {
			actions = []string{"complete_risk_mitigation", "reassess_risks"}
		}
		if m.LifeSupportPlan != nil {
			actions = []string{"submit_life_support_plan"}
		}
	case StatusPlanReview:
		actions = []string{"review_life_support_plan"}
	case StatusEquipmentVerification:
		actions = []string{"verify_equipment"}
	case StatusDrillPending:
		actions = []string{"record_drill"}
	case StatusRemediation:
		actions = []string{"record_remediation", "record_retest"}
	case StatusReadyForRelease, StatusReleaseRejected:
		actions = []string{"release"}
	case StatusSigned:
		actions = []string{"archive"}
	}
	if (m.Status == StatusDrillPending || m.Status == StatusReadyForRelease || m.Status == StatusReleaseRejected) && len(expiredEquipmentCodes(m)) > 0 {
		actions = append(actions, "refresh_equipment_evidence")
	}
	return actions
}
