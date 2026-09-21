package ingest

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/dhananjaya/flakeguard/internal/repository"
)

type Handler struct {
	repo *repository.Repository
}

func NewHandler(repo *repository.Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var report Report
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}

	if err := report.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	runs := make([]repository.TestRun, len(report.Tests))
	for i, t := range report.Tests {
		runs[i] = repository.TestRun{
			Repo:       report.Repo,
			Branch:     report.Branch,
			CommitSHA:  report.CommitSHA,
			TestName:   t.TestName,
			Status:     t.Status,
			DurationMS: t.DurationMS,
			CIRunID:    report.CIRunID,
			RanAt:      t.RanAt,
		}
	}

	if err := h.repo.RecordRuns(r.Context(), runs); err != nil {
		log.Printf("recording test runs: %v", err)
		http.Error(w, "failed to record test runs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int{"recorded": len(runs)})
}
