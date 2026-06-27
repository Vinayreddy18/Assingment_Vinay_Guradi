package negotiation

import (
	"context"
	"strings"
	"testing"

	"github.com/pranav/samadhan/internal/domain"
	"github.com/pranav/samadhan/internal/llm"
)

// analyzedDispute builds a dispute with a hand-set analysis so the negotiation
// math is predictable and independent of the analysis engine.
func analyzedDispute(floor, ceiling, recommended int64) *domain.Dispute {
	return &domain.Dispute{
		ID:          "T-NEG",
		Category:    domain.CategoryChequeBounce,
		Claimant:    domain.Party{Name: "Claimant"},
		Respondent:  domain.Party{Name: "Respondent"},
		ClaimAmount: 250000,
		Currency:    "INR",
		Status:      domain.StatusAnalyzed,
		Analysis: &domain.CaseAnalysis{
			ClaimStrength:         0.8,
			ClaimantReservation:   floor,
			RespondentReservation: ceiling,
			ZOPAExists:            ceiling >= floor,
			ZOPALow:               floor,
			ZOPAHigh:              ceiling,
			ZOPAWidth:             ceiling - floor,
			RecommendedSettlement: recommended,
			FairnessWeight:        0.5,
			Provider:              "offline-model",
		},
	}
}

func newEngine() *Engine { return New(llm.NewMock()) }

func TestSubmitRound_SettleOnCross(t *testing.T) {
	e := newEngine()
	d := analyzedDispute(100000, 200000, 150000)
	if err := e.Start(d, 3); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Claimant asks 140k, respondent offers 150k: the offers cross.
	round, err := e.SubmitRound(context.Background(), d, 140000, 150000)
	if err != nil {
		t.Fatalf("SubmitRound: %v", err)
	}
	if round.Outcome != domain.OutcomeSettled {
		t.Fatalf("expected settled, got %s", round.Outcome)
	}
	// Midpoint of 140k and 150k is 145k, already a multiple of 500.
	if round.SettledAmount != 145000 {
		t.Errorf("expected midpoint 145000, got %d", round.SettledAmount)
	}
	if round.SettledAmount%500 != 0 {
		t.Errorf("settlement not rounded to 500: %d", round.SettledAmount)
	}
	if d.Negotiation.Status != "settled" {
		t.Errorf("negotiation status = %s, want settled", d.Negotiation.Status)
	}
}

func TestSubmitRound_EscalatesAfterMaxRounds(t *testing.T) {
	e := newEngine()
	d := analyzedDispute(100000, 200000, 150000)
	if err := e.Start(d, 2); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Round 1: a gap remains -> continue with nudges.
	r1, err := e.SubmitRound(context.Background(), d, 190000, 110000)
	if err != nil {
		t.Fatalf("round 1: %v", err)
	}
	if r1.Outcome != domain.OutcomeContinue {
		t.Fatalf("round 1 expected continue, got %s", r1.Outcome)
	}

	// Round 2 is the last; a gap remains -> escalate.
	r2, err := e.SubmitRound(context.Background(), d, 188000, 112000)
	if err != nil {
		t.Fatalf("round 2: %v", err)
	}
	if r2.Outcome != domain.OutcomeEscalated {
		t.Fatalf("round 2 expected escalated, got %s", r2.Outcome)
	}
	if d.Status != domain.StatusEscalated {
		t.Errorf("dispute status = %s, want escalated", d.Status)
	}
}

func TestSubmitRound_NudgesAreConfidential(t *testing.T) {
	e := newEngine()
	d := analyzedDispute(100000, 200000, 150000)
	if err := e.Start(d, 3); err != nil {
		t.Fatalf("Start: %v", err)
	}

	const claimantAsk = 185000
	const respondentPay = 105000
	round, err := e.SubmitRound(context.Background(), d, claimantAsk, respondentPay)
	if err != nil {
		t.Fatalf("SubmitRound: %v", err)
	}
	if round.Outcome != domain.OutcomeContinue {
		t.Fatalf("expected continue, got %s", round.Outcome)
	}
	if len(round.Nudges) != 2 {
		t.Fatalf("expected one nudge per party, got %d", len(round.Nudges))
	}

	var claimantNudge, respondentNudge domain.Nudge
	for _, n := range round.Nudges {
		switch n.Party {
		case domain.PartyClaimant:
			claimantNudge = n
		case domain.PartyRespondent:
			respondentNudge = n
		}
	}

	// The core confidentiality guarantee: a party's nudge must never contain
	// the other party's confidential figure.
	respondentFigure := domain.FormatINR(respondentPay)
	if strings.Contains(claimantNudge.Message, respondentFigure) {
		t.Errorf("claimant nudge leaked the respondent's offer %q: %s", respondentFigure, claimantNudge.Message)
	}
	claimantFigure := domain.FormatINR(claimantAsk)
	if strings.Contains(respondentNudge.Message, claimantFigure) {
		t.Errorf("respondent nudge leaked the claimant's ask %q: %s", claimantFigure, respondentNudge.Message)
	}

	// A nudge should pull a party toward the fair figure: the claimant's
	// suggestion should not exceed its ask, the respondent's should not be
	// below its offer.
	if claimantNudge.SuggestedOffer > claimantAsk {
		t.Errorf("claimant nudge suggested moving up: %d > %d", claimantNudge.SuggestedOffer, claimantAsk)
	}
	if respondentNudge.SuggestedOffer < respondentPay {
		t.Errorf("respondent nudge suggested moving down: %d < %d", respondentNudge.SuggestedOffer, respondentPay)
	}
}

func TestSimulateParties_Converges(t *testing.T) {
	e := newEngine()
	d := analyzedDispute(100000, 200000, 150000)

	err := e.SimulateParties(context.Background(), d, SimOptions{
		ClaimantStart:   250000,
		RespondentStart: 60000,
		ConcessionRate:  0.5,
		MaxRounds:       8,
	})
	if err != nil {
		t.Fatalf("SimulateParties: %v", err)
	}
	if d.Negotiation.Status != "settled" {
		t.Fatalf("expected a settlement, got status %s after %d rounds",
			d.Negotiation.Status, len(d.Negotiation.Rounds))
	}
	amt := d.Negotiation.SettledAmount
	if amt < d.Analysis.ZOPALow || amt > d.Analysis.ZOPAHigh {
		t.Errorf("settled amount %d outside the zone [%d, %d]", amt, d.Analysis.ZOPALow, d.Analysis.ZOPAHigh)
	}
}

func TestStart_RequiresAnalysis(t *testing.T) {
	e := newEngine()
	d := &domain.Dispute{ID: "T", Category: domain.CategoryGeneric, ClaimAmount: 100000}
	if err := e.Start(d, 3); err == nil {
		t.Error("expected Start to fail without an analysis")
	}
}
