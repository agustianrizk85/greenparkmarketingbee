package service

import (
	"sort"
	"time"

	"marketingflow/internal/contentplan"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"
)

// SourceContentPlan marks work items ingested from the Content Plan spreadsheet.
const SourceContentPlan = "content-plan"

// ContentPlanService turns parsed Content Plan rows into work items.
type ContentPlanService struct {
	items *repository.WorkItemRepository
}

func NewContentPlanService(items *repository.WorkItemRepository) *ContentPlanService {
	return &ContentPlanService{items: items}
}

// PreviewRow is one planned item annotated with whether the sync would create it.
type PreviewRow struct {
	contentplan.Planned
	IsNew bool `json:"is_new"`
}

// PreviewResult is the dry-run shown before approval.
type PreviewResult struct {
	Summary  contentplan.Summary `json:"summary"`
	NewCount int                 `json:"new_count"`
	Existing int                 `json:"existing"`
	Rows     []PreviewRow        `json:"rows"`
}

// Preview parses the workbook and reports what a sync would do, without writing.
func (s *ContentPlanService) Preview(tabs map[string][][]string) (*PreviewResult, error) {
	planned, summary := contentplan.Parse(tabs)
	existing, err := s.items.ExistingSourceKeys()
	if err != nil {
		return nil, err
	}
	res := &PreviewResult{Summary: summary, Rows: make([]PreviewRow, 0, len(planned))}
	for _, p := range planned {
		isNew := !existing[p.SourceKey]
		if isNew {
			res.NewCount++
		} else {
			res.Existing++
		}
		res.Rows = append(res.Rows, PreviewRow{Planned: p, IsNew: isNew})
	}
	sortRows(res.Rows)
	return res, nil
}

// ApproveResult summarises an applied sync.
type ApproveResult struct {
	Created  int       `json:"created"`
	Updated  int       `json:"updated"`
	Skipped  int       `json:"skipped"`
	Total    int       `json:"total"`
	SyncedAt time.Time `json:"synced_at"`
}

// Approve parses the workbook and upserts a work item per planned item: new
// items get the seeded checklist; already-imported items have their descriptive
// fields (title/brief/caption/content type/date/source tab) refreshed so edits
// in the sheet propagate — their steps and progress are left untouched.
func (s *ContentPlanService) Approve(tabs map[string][][]string, by uint) (*ApproveResult, error) {
	planned, _ := contentplan.Parse(tabs)
	existing, err := s.items.ExistingSourceKeys()
	if err != nil {
		return nil, err
	}
	res := &ApproveResult{Total: len(planned), SyncedAt: time.Now().UTC()}
	seen := map[string]bool{}
	for _, p := range planned {
		if seen[p.SourceKey] {
			res.Skipped++ // duplicate row within this workbook
			continue
		}
		seen[p.SourceKey] = true

		if existing[p.SourceKey] {
			// Refresh descriptive fields only (keep steps/stage/progress, keep alur).
			if err := s.items.UpdateSyncedMeta(p.SourceKey, map[string]any{
				"title":        trim180(p.Title),
				"project":      p.Project,
				"source_tab":   p.SourceTab,
				"content_type": p.ContentType,
				"planned_date": p.Date,
				"brief":        p.Brief,
				"caption":      p.Caption,
			}); err != nil {
				return nil, err
			}
			res.Updated++
			continue
		}

		anchor := time.Now().UTC()
		if p.Date != nil {
			anchor = *p.Date
		}
		item := &model.WorkItem{
			Title:       trim180(p.Title),
			Alur:        p.Alur,
			Project:     p.Project,
			Stage:       model.StageBrief,
			CreatedBy:   by,
			Source:      SourceContentPlan,
			SourceKey:   p.SourceKey,
			SourceTab:   p.SourceTab,
			ContentType: p.ContentType,
			PlannedDate: p.Date,
			Brief:       p.Brief,
			Caption:     p.Caption,
		}
		steps := BuildSteps(p.Alur, anchor)
		if err := s.items.CreateWithSteps(item, steps); err != nil {
			return nil, err
		}
		res.Created++
	}
	return res, nil
}

func trim180(s string) string {
	r := []rune(s)
	if len(r) > 180 {
		return string(r[:180])
	}
	return s
}

// sortRows orders preview rows by project then date then title for a stable UI.
func sortRows(rows []PreviewRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Project != rows[j].Project {
			return rows[i].Project < rows[j].Project
		}
		di, dj := "", ""
		if rows[i].Date != nil {
			di = rows[i].Date.Format("2006-01-02")
		}
		if rows[j].Date != nil {
			dj = rows[j].Date.Format("2006-01-02")
		}
		if di != dj {
			return di < dj
		}
		return rows[i].Title < rows[j].Title
	})
}
