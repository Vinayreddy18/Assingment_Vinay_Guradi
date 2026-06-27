package api

import (
	"net/http"

	"github.com/pranav/samadhan/internal/negotiation"
)

// Handlers holds the HTTP handlers and the service they delegate to.
type Handlers struct {
	svc *Service
}

// NewHandlers constructs the handler set.
func NewHandlers(svc *Service) *Handlers { return &Handlers{svc: svc} }

// health is a liveness probe.
func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// createDispute handles POST /api/v1/disputes.
func (h *Handlers) createDispute(w http.ResponseWriter, r *http.Request) {
	var in CreateDisputeInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	d, err := h.svc.CreateDispute(in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

// listDisputes handles GET /api/v1/disputes.
func (h *Handlers) listDisputes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"disputes": h.svc.List()})
}

// getDispute handles GET /api/v1/disputes/{id}.
func (h *Handlers) getDispute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := h.svc.Get(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// analyzeDispute handles POST /api/v1/disputes/{id}/analyze.
func (h *Handlers) analyzeDispute(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := h.svc.Analyze(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// offersRequest is the body for a single confidential bidding round.
type offersRequest struct {
	ClaimantAsk   int64 `json:"claimant_ask"`
	RespondentPay int64 `json:"respondent_pay"`
}

// offersResponse returns the processed round plus the (possibly updated)
// dispute so the client can re-render state in one round-trip.
type offersResponse struct {
	Round   any `json:"round"`
	Dispute any `json:"dispute"`
}

// submitOffers handles POST /api/v1/disputes/{id}/offers.
func (h *Handlers) submitOffers(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req offersRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	round, d, err := h.svc.SubmitOffers(r.Context(), id, req.ClaimantAsk, req.RespondentPay)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, offersResponse{Round: round, Dispute: d})
}

// simulateRequest configures a synthetic negotiation. All fields are optional;
// the service/engine fall back to analysis-derived defaults when zero.
type simulateRequest struct {
	ClaimantStart   int64   `json:"claimant_start"`
	RespondentStart int64   `json:"respondent_start"`
	ConcessionRate  float64 `json:"concession_rate"`
	MaxRounds       int     `json:"max_rounds"`
}

// simulate handles POST /api/v1/disputes/{id}/simulate.
func (h *Handlers) simulate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req simulateRequest
	// Body is optional for simulate; ignore EOF on an empty body.
	if r.ContentLength != 0 {
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, err)
			return
		}
	}
	opts := negotiation.SimOptions{
		ClaimantStart:   req.ClaimantStart,
		RespondentStart: req.RespondentStart,
		ConcessionRate:  req.ConcessionRate,
		MaxRounds:       req.MaxRounds,
	}
	d, err := h.svc.Simulate(r.Context(), id, opts)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// acceptRecommended handles POST /api/v1/disputes/{id}/accept.
func (h *Handlers) acceptRecommended(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	d, err := h.svc.AcceptRecommended(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}
