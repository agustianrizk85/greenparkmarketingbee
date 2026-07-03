package service

import (
	"testing"
	"time"

	"marketingflow/internal/model"
)

// Every alur must have a catalog, an unknown alur must not, and each catalog's
// step codes must be unique and tagged with the owning alur — the whole workflow
// engine is driven off these tables, so a typo here silently breaks seeding.
func TestCatalogFor(t *testing.T) {
	for _, alur := range []model.Alur{model.AlurHardsell, model.AlurVideoAd, model.AlurCarousel, model.AlurReels} {
		steps := CatalogFor(alur)
		if len(steps) == 0 {
			t.Fatalf("alur %q: expected a non-empty catalog", alur)
		}
		seen := map[string]bool{}
		for i, st := range steps {
			if st.Code == "" {
				t.Errorf("alur %q step %d: empty code", alur, i)
			}
			if seen[st.Code] {
				t.Errorf("alur %q: duplicate step code %q", alur, st.Code)
			}
			seen[st.Code] = true
			if model.Alur(st.Alur) != alur {
				t.Errorf("step %q: Alur=%q, want %q", st.Code, st.Alur, alur)
			}
		}
	}

	if got := CatalogFor(model.Alur("Z")); got != nil {
		t.Errorf("unknown alur: got %d steps, want nil", len(got))
	}
}

// AlurLabels backs the UI dropdown; every alur code must have a label.
func TestAlurLabelsComplete(t *testing.T) {
	for _, code := range []string{"A", "B", "C", "D"} {
		if AlurLabels[code] == "" {
			t.Errorf("AlurLabels missing entry for %q", code)
		}
	}
}

// BuildSteps must produce one step per template, number them 1..n in order, and
// anchor each SLA'd step's due date to anchor+SLA. A zero anchor must leave due
// dates nil (used so manual creation and sheet sync differ only by anchor).
func TestBuildSteps(t *testing.T) {
	anchor := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	steps := BuildSteps(model.AlurHardsell, anchor)

	templates := CatalogFor(model.AlurHardsell)
	if len(steps) != len(templates) {
		t.Fatalf("BuildSteps: got %d steps, want %d", len(steps), len(templates))
	}

	for i, st := range steps {
		if st.Sequence != i+1 {
			t.Errorf("step %q: Sequence=%d, want %d", st.Code, st.Sequence, i+1)
		}
		if st.Status != model.StatusPending {
			t.Errorf("step %q: Status=%q, want pending", st.Code, st.Status)
		}
		if st.SLADays > 0 {
			if st.DueDate == nil {
				t.Errorf("step %q: expected a due date for SLA %d", st.Code, st.SLADays)
				continue
			}
			want := anchor.AddDate(0, 0, st.SLADays)
			if !st.DueDate.Equal(want) {
				t.Errorf("step %q: DueDate=%v, want %v", st.Code, st.DueDate, want)
			}
		}
	}
}

func TestBuildStepsZeroAnchorHasNoDueDates(t *testing.T) {
	for _, st := range BuildSteps(model.AlurReels, time.Time{}) {
		if st.DueDate != nil {
			t.Errorf("step %q: DueDate set on zero anchor (%v)", st.Code, st.DueDate)
		}
	}
}
