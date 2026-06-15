package service

import (
	"encoding/json"
	"errors"
	"time"

	"marketingflow/internal/dto"
	"marketingflow/internal/model"
	"marketingflow/internal/repository"

	"gorm.io/datatypes"
)

var (
	ErrBudgetRequired = errors.New("nominal budget wajib diisi sebelum step ini diselesaikan")
	ErrApprovalRole   = errors.New("hanya Kepala Departemen Marketing yang dapat menyelesaikan step approval")
)

type StepService struct {
	steps     *repository.StepRepository
	workItems *WorkItemService
}

func NewStepService(steps *repository.StepRepository, workItems *WorkItemService) *StepService {
	return &StepService{steps: steps, workItems: workItems}
}

func (s *StepService) Get(id uint) (*model.WorkStep, error) {
	return s.steps.FindByID(id)
}

// Mine returns the steps owned by a given PIC position (Copywriter, Talent, …).
func (s *StepService) Mine(position string) ([]repository.MineStep, error) {
	if position == "" {
		return []repository.MineStep{}, nil
	}
	return s.steps.ByOwner(position)
}

// Update applies a partial update and enforces the flowchart's completion rules,
// then recomputes the parent work item stage.
func (s *StepService) Update(id uint, req dto.UpdateStepRequest, actor uint, role model.Role) (*model.WorkStep, error) {
	step, err := s.steps.FindByID(id)
	if err != nil {
		return nil, err
	}

	if req.BudgetAmount != nil {
		step.BudgetAmount = *req.BudgetAmount
	}
	if req.Notes != nil {
		step.Notes = *req.Notes
	}
	if req.Metadata != nil {
		raw, err := json.Marshal(req.Metadata)
		if err != nil {
			return nil, err
		}
		step.Metadata = datatypes.JSON(raw)
	}
	if req.SLADays != nil {
		step.SLADays = *req.SLADays
	}
	if req.DueDate != nil {
		step.DueDate = req.DueDate
	}

	if req.Status != nil {
		if err := s.applyStatus(step, *req.Status, actor, role); err != nil {
			return nil, err
		}
	}

	if err := s.steps.Save(step); err != nil {
		return nil, err
	}
	if err := s.workItems.RecomputeStage(step.WorkItemID); err != nil {
		return nil, err
	}
	return step, nil
}

// applyStatus validates the completion guardrails before changing status.
func (s *StepService) applyStatus(step *model.WorkStep, status model.StepStatus, actor uint, role model.Role) error {
	if status == model.StatusDone {
		if step.RequiresBudget && step.BudgetAmount <= 0 {
			return ErrBudgetRequired
		}
		if step.IsApproval && role != model.RoleKadep {
			return ErrApprovalRole
		}
		now := time.Now().UTC()
		step.CompletedAt = &now
		step.CompletedBy = &actor
	} else {
		step.CompletedAt = nil
		step.CompletedBy = nil
	}
	step.Status = status
	return nil
}
