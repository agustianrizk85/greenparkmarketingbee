package service

import (
	"time"

	"marketingflow/internal/dto"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"
)

type WorkItemService struct {
	items *repository.WorkItemRepository
	steps *repository.StepRepository
}

func NewWorkItemService(items *repository.WorkItemRepository, steps *repository.StepRepository) *WorkItemService {
	return &WorkItemService{items: items, steps: steps}
}

// Create persists a new work item and seeds its checklist from the alur catalog.
func (s *WorkItemService) Create(req dto.CreateWorkItemRequest, createdBy uint) (*model.WorkItem, error) {
	item := &model.WorkItem{
		Title:     req.Title,
		Alur:      req.Alur,
		Project:   req.Project,
		Stage:     model.StageBrief,
		CreatedBy: createdBy,
	}

	base := time.Now().UTC()
	templates := CatalogFor(req.Alur)
	steps := make([]model.WorkStep, 0, len(templates))
	for i, t := range templates {
		step := t.toModel(0, i+1)
		if step.SLADays > 0 {
			due := base.AddDate(0, 0, step.SLADays)
			step.DueDate = &due
		}
		steps = append(steps, step)
	}

	if err := s.items.CreateWithSteps(item, steps); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *WorkItemService) List() ([]model.WorkItem, error) {
	return s.items.List()
}

func (s *WorkItemService) Get(id uint) (*model.WorkItem, error) {
	return s.items.FindByID(id)
}

// Progress computes the checklist completion summary for the dashboard.
func (s *WorkItemService) Progress(id uint) (*dto.WorkItemProgress, error) {
	counts, err := s.steps.CountByStatus(id)
	if err != nil {
		return nil, err
	}
	var total, done int64
	for status, n := range counts {
		total += n
		if status == model.StatusDone {
			done += n
		}
	}
	pct := 0
	if total > 0 {
		pct = int(done * 100 / total)
	}
	return &dto.WorkItemProgress{
		WorkItemID: id,
		Total:      total,
		Done:       done,
		Percentage: pct,
		ByStatus:   counts,
	}, nil
}

// RecomputeStage advances the work item stage to the phase of the earliest
// not-done step (or "done" when every step is complete). Called after a step
// status change so the dashboard stepper reflects real progress.
func (s *WorkItemService) RecomputeStage(workItemID uint) error {
	steps, err := s.steps.ByWorkItem(workItemID)
	if err != nil {
		return err
	}
	stage := model.StageDone
	for _, st := range steps {
		if st.Status != model.StatusDone {
			stage = model.WorkStage(st.Phase)
			break
		}
	}
	return s.items.UpdateStage(workItemID, stage)
}
