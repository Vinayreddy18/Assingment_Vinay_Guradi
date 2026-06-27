package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/pranav/samadhan/internal/analysis"
	"github.com/pranav/samadhan/internal/domain"
	"github.com/pranav/samadhan/internal/drafting"
	"github.com/pranav/samadhan/internal/negotiation"
	"github.com/pranav/samadhan/internal/store"
)

// Service is the application layer. It coordinates the store and the three
// engines (analysis, negotiation, drafting) into the use-cases the HTTP layer
// exposes, and is the only place that mutates dispute state.
type Service struct {
	store      *store.Store
	analyzer   *analysis.Engine
	negotiator *negotiation.Engine
	drafter    *drafting.Drafter
	maxRounds  int
	log        *slog.Logger
}

// NewService wires the application layer.
func NewService(st *store.Store, an *analysis.Engine, ne *negotiation.Engine, dr *drafting.Drafter, maxRounds int, log *slog.Logger) *Service {
	return &Service{store: st, analyzer: an, negotiator: ne, drafter: dr, maxRounds: maxRounds, log: log}
}

// CreateDisputeInput is the payload to open a dispute.
type CreateDisputeInput struct {
	Category           string            `json:"category"`
	Title              string            `json:"title"`
	Claimant           domain.Party      `json:"claimant"`
	Respondent         domain.Party      `json:"respondent"`
	ClaimAmount        int64             `json:"claim_amount"`
	Currency           string            `json:"currency"`
	Narrative          string            `json:"narrative"`
	RespondentResponse string            `json:"respondent_response"`
	Documents          []domain.Document `json:"documents"`
}

// CreateDispute validates and stores a new dispute.
func (s *Service) CreateDispute(in CreateDisputeInput) (*domain.Dispute, error) {
	cat := domain.Category(strings.TrimSpace(in.Category))
	if !cat.Valid() {
		return nil, fmt.Errorf("%w: unknown category %q", domain.ErrInvalidInput, in.Category)
	}
	if in.ClaimAmount <= 0 {
		return nil, fmt.Errorf("%w: claim_amount must be positive", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(in.Claimant.Name) == "" || strings.TrimSpace(in.Respondent.Name) == "" {
		return nil, fmt.Errorf("%w: claimant and respondent names are required", domain.ErrInvalidInput)
	}
	currency := in.Currency
	if currency == "" {
		currency = "INR"
	}
	d := &domain.Dispute{
		Category:           cat,
		Title:              strings.TrimSpace(in.Title),
		Claimant:           in.Claimant,
		Respondent:         in.Respondent,
		ClaimAmount:        in.ClaimAmount,
		Currency:           currency,
		Narrative:          in.Narrative,
		RespondentResponse: in.RespondentResponse,
		Documents:          in.Documents,
		Status:             domain.StatusIntake,
	}
	return s.store.Create(d), nil
}

// Get returns a dispute by ID.
func (s *Service) Get(id string) (*domain.Dispute, error) { return s.store.Get(id) }

// List returns all disputes.
func (s *Service) List() []*domain.Dispute { return s.store.List() }

// Analyze runs the settlement-intelligence engine and stores the result.
func (s *Service) Analyze(ctx context.Context, id string) (*domain.Dispute, error) {
	d, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	a, err := s.analyzer.Analyze(ctx, d)
	if err != nil {
		return nil, err
	}
	d.Analysis = a
	if d.Status == domain.StatusIntake {
		d.Status = domain.StatusAnalyzed
	}
	if err := s.store.Update(d); err != nil {
		return nil, err
	}
	s.log.Info("analysed dispute", "id", d.ID, "zopa", a.ZOPAExists, "recommended", a.RecommendedSettlement, "provider", a.Provider)
	return d, nil
}

// SubmitOffers processes one round of confidential offers, auto-starting the
// negotiation if needed and finalising the settlement when offers cross.
func (s *Service) SubmitOffers(ctx context.Context, id string, claimantAsk, respondentPay int64) (domain.Round, *domain.Dispute, error) {
	d, err := s.store.Get(id)
	if err != nil {
		return domain.Round{}, nil, err
	}
	if d.Analysis == nil {
		return domain.Round{}, nil, domain.ErrNotAnalyzed
	}
	if d.Negotiation == nil {
		if err := s.negotiator.Start(d, s.maxRounds); err != nil {
			return domain.Round{}, nil, err
		}
	}
	round, err := s.negotiator.SubmitRound(ctx, d, claimantAsk, respondentPay)
	if err != nil {
		return domain.Round{}, nil, err
	}
	if round.Outcome == domain.OutcomeSettled {
		if err := s.finalize(ctx, d, round.SettledAmount, "negotiated"); err != nil {
			return domain.Round{}, nil, err
		}
	}
	if err := s.store.Update(d); err != nil {
		return domain.Round{}, nil, err
	}
	return round, d, nil
}

// Simulate runs a synthetic negotiation end to end (demo/testing) and finalises
// any resulting settlement.
func (s *Service) Simulate(ctx context.Context, id string, opts negotiation.SimOptions) (*domain.Dispute, error) {
	d, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if d.Analysis == nil {
		return nil, domain.ErrNotAnalyzed
	}
	if err := s.negotiator.SimulateParties(ctx, d, opts); err != nil {
		return nil, err
	}
	if d.Negotiation != nil && d.Negotiation.Status == "settled" && d.Settlement == nil {
		if err := s.finalize(ctx, d, d.Negotiation.SettledAmount, "negotiated"); err != nil {
			return nil, err
		}
	}
	if err := s.store.Update(d); err != nil {
		return nil, err
	}
	return d, nil
}

// AcceptRecommended settles the dispute at the engine's recommended figure as a
// mediator's proposal both parties accept. This is the fast path for the large
// share of cases where both sides simply take the neutral number.
func (s *Service) AcceptRecommended(ctx context.Context, id string) (*domain.Dispute, error) {
	d, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if d.Analysis == nil {
		return nil, domain.ErrNotAnalyzed
	}
	if d.Settlement != nil {
		return nil, domain.ErrAlreadySettled
	}
	amount := d.Analysis.RecommendedSettlement
	if amount <= 0 {
		return nil, fmt.Errorf("%w: no positive recommended settlement to accept", domain.ErrInvalidInput)
	}
	if err := s.finalize(ctx, d, amount, "mediator_proposal"); err != nil {
		return nil, err
	}
	if d.Negotiation != nil {
		d.Negotiation.Status = "settled"
		d.Negotiation.SettledAmount = amount
	}
	if err := s.store.Update(d); err != nil {
		return nil, err
	}
	return d, nil
}

// finalize drafts the agreement and marks the dispute settled.
func (s *Service) finalize(ctx context.Context, d *domain.Dispute, amount int64, method string) error {
	settlement, err := s.drafter.Draft(ctx, d, amount, method)
	if err != nil {
		return err
	}
	d.Settlement = settlement
	d.Status = domain.StatusSettled
	s.log.Info("settled dispute", "id", d.ID, "amount", amount, "method", method)
	return nil
}
