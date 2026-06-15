package service

import (
	"fmt"
	"time"

	"marketingflow/internal/repository"
)

// WarningSeverity ranks an early-warning item.
type WarningSeverity string

const (
	SeverityCritical WarningSeverity = "critical" // overdue
	SeverityWarning  WarningSeverity = "warning"  // due soon
	SeverityInfo     WarningSeverity = "info"     // missing required input
)

// Warning is one early-warning entry for the dashboard.
type Warning struct {
	WorkItemID    uint            `json:"work_item_id"`
	WorkItemTitle string          `json:"work_item_title"`
	StepID        uint            `json:"step_id"`
	StepCode      string          `json:"step_code"`
	StepName      string          `json:"step_name"`
	Owner         string          `json:"owner"`
	Severity      WarningSeverity `json:"severity"`
	Message       string          `json:"message"`
	DueDate       *time.Time      `json:"due_date"`
}

// DashboardService produces the early-warning feed across all work items. The
// rules below are deterministic; an AI provider can later enrich Message.
type DashboardService struct {
	steps *repository.StepRepository
}

func NewDashboardService(steps *repository.StepRepository) *DashboardService {
	return &DashboardService{steps: steps}
}

const dueSoonWindow = 24 * time.Hour

// EarlyWarnings scans all open steps and flags SLA breaches and missing budget.
func (s *DashboardService) EarlyWarnings() ([]Warning, error) {
	open, err := s.steps.OpenSteps()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	warnings := make([]Warning, 0)

	for _, st := range open {
		base := Warning{
			WorkItemID:    st.WorkItemID,
			WorkItemTitle: st.WorkItemTitle,
			StepID:        st.ID,
			StepCode:      st.Code,
			StepName:      st.Name,
			Owner:         st.Owner,
			DueDate:       st.DueDate,
		}

		// SLA rules.
		if st.DueDate != nil {
			switch {
			case now.After(*st.DueDate):
				days := int(now.Sub(*st.DueDate).Hours() / 24)
				w := base
				w.Severity = SeverityCritical
				w.Message = fmt.Sprintf("Terlambat %d hari dari deadline SLA.", days)
				warnings = append(warnings, w)
			case st.DueDate.Sub(now) <= dueSoonWindow:
				w := base
				w.Severity = SeverityWarning
				w.Message = "Mendekati deadline (≤1 hari)."
				warnings = append(warnings, w)
			}
		}

		// Missing-input rule (blocks completion later).
		if st.RequiresBudget && st.BudgetAmount <= 0 {
			w := base
			w.Severity = SeverityInfo
			w.Message = fmt.Sprintf("%s belum diisi.", budgetLabel(st.BudgetLabel))
			warnings = append(warnings, w)
		}
	}
	return warnings, nil
}

func budgetLabel(label string) string {
	if label == "" {
		return "Budget"
	}
	return label
}
