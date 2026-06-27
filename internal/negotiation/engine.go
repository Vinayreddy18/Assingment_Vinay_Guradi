// Package negotiation runs the structured bargaining protocol that sits on top
// of the settlement intelligence. The protocol is a double-blind bid: each
// party submits a confidential figure and the engine reveals only the outcome,
// never the opposing number. This is a proven Online Dispute Resolution
// technique — it removes anchoring and gaming — and here it is augmented with
// neutral, BATNA-grounded nudges and an explicit escalation tripwire so the
// system hands off to a human the moment it stops adding value.
package negotiation

import (
	"context"
	"fmt"
	"time"

	"github.com/pranav/samadhan/internal/domain"
	"github.com/pranav/samadhan/internal/llm"
)

const defaultMaxRounds = 3

// Engine drives negotiations.
type Engine struct {
	model llm.Provider
}

// New builds a negotiation engine backed by the given model provider.
func New(model llm.Provider) *Engine { return &Engine{model: model} }

// Start initialises the negotiation state on a dispute. The dispute must have
// been analysed first — the bargaining is meaningless without a fair range.
func (e *Engine) Start(d *domain.Dispute, maxRounds int) error {
	if d.Analysis == nil {
		return domain.ErrNotAnalyzed
	}
	if d.Settlement != nil {
		return domain.ErrAlreadySettled
	}
	if maxRounds <= 0 {
		maxRounds = defaultMaxRounds
	}
	d.Negotiation = &domain.Negotiation{
		Protocol:     "double_blind_bid",
		MaxRounds:    maxRounds,
		CurrentRound: 0,
		Rounds:       nil,
		Status:       "open",
	}
	d.Status = domain.StatusNegotiating
	return nil
}

// SubmitRound processes one round of confidential offers and returns the round
// outcome. If the offers cross, the matter settles at the midpoint (the
// standard blind-bid rule). If a gap remains, each party receives a private
// nudge grounded only in their own alternative to settling. When the round
// budget is exhausted without a deal, the negotiation escalates to a human
// neutral.
func (e *Engine) SubmitRound(ctx context.Context, d *domain.Dispute, claimantAsk, respondentPay int64) (domain.Round, error) {
	if d.Negotiation == nil {
		return domain.Round{}, domain.ErrNotNegotiating
	}
	if d.Negotiation.Status != "open" {
		return domain.Round{}, fmt.Errorf("%w: negotiation is %s", domain.ErrNotNegotiating, d.Negotiation.Status)
	}
	if claimantAsk < 0 || respondentPay < 0 {
		return domain.Round{}, fmt.Errorf("%w: offers must be non-negative", domain.ErrInvalidInput)
	}

	n := d.Negotiation
	roundNo := n.CurrentRound + 1
	gap := claimantAsk - respondentPay

	round := domain.Round{
		Number:        roundNo,
		ClaimantAsk:   claimantAsk,
		RespondentPay: respondentPay,
		Gap:           gap,
		At:            time.Now().UTC(),
	}

	switch {
	case gap <= 0:
		// Offers cross: settle at the midpoint.
		settled := roundTo((claimantAsk+respondentPay)/2, 500)
		round.Outcome = domain.OutcomeSettled
		round.SettledAmount = settled
		round.Note = "Confidential offers crossed. Settled at the midpoint of the two figures."
		n.Status = "settled"
		n.SettledAmount = settled

	case roundNo >= n.MaxRounds:
		// Out of rounds with a gap remaining: escalate.
		round.Outcome = domain.OutcomeEscalated
		round.Note = fmt.Sprintf("A gap of %s remains after the final round. Escalating to a human neutral for evaluative mediation.", domain.FormatINR(gap))
		n.Status = "escalated"
		d.Status = domain.StatusEscalated

	default:
		// Gap remains, rounds left: nudge both sides privately.
		round.Outcome = domain.OutcomeContinue
		round.Note = fmt.Sprintf("A gap of %s remains. Each party has been sent a private, neutral nudge.", domain.FormatINR(gap))
		round.Nudges = []domain.Nudge{
			e.nudge(ctx, d, domain.PartyClaimant, d.Analysis.ClaimantReservation, claimantAsk, roundNo),
			e.nudge(ctx, d, domain.PartyRespondent, d.Analysis.RespondentReservation, respondentPay, roundNo),
		}
	}

	n.Rounds = append(n.Rounds, round)
	n.CurrentRound = roundNo
	return round, nil
}

// nudge asks the model for a neutral message for one party. It is resilient:
// if the model call fails, a deterministic fallback message is used so a
// nudge failure never breaks the round.
func (e *Engine) nudge(ctx context.Context, d *domain.Dispute, party domain.OfferParty, alternative, currentOffer int64, round int) domain.Nudge {
	facts := map[string]any{
		"party":            string(party),
		"their_alternative": alternative,
		"current_offer":    currentOffer,
		"recommended":      d.Analysis.RecommendedSettlement,
		"round":            round,
		"category":         string(d.Category),
	}

	type nudgeResult struct {
		Message        string `json:"message"`
		SuggestedOffer int64  `json:"suggested_offer"`
	}

	out, err := llm.CompleteJSON[nudgeResult](ctx, e.model, llm.NudgeRequest(facts))
	if err != nil || out.Message == "" {
		return domain.Nudge{
			Party:          party,
			Message:        fallbackNudge(party, alternative, d.Analysis.RecommendedSettlement),
			SuggestedOffer: d.Analysis.RecommendedSettlement,
			Round:          round,
		}
	}
	return domain.Nudge{
		Party:          party,
		Message:        out.Message,
		SuggestedOffer: out.SuggestedOffer,
		Round:          round,
	}
}

func fallbackNudge(party domain.OfferParty, alternative, recommended int64) string {
	if party == domain.PartyRespondent {
		return fmt.Sprintf("Your modelled exposure if this is adjudicated is around %s, before your own costs and the delay. A settlement near %s removes that risk on terms you control.",
			domain.FormatINR(alternative), domain.FormatINR(recommended))
	}
	return fmt.Sprintf("The modelled present value of pursuing this in litigation is around %s, after time, cost and recovery risk. A settlement at or above that — near %s — is the rational, immediate outcome.",
		domain.FormatINR(alternative), domain.FormatINR(recommended))
}

// SimOptions configures a synthetic negotiation used for demos and tests.
type SimOptions struct {
	ClaimantStart  int64   // claimant's opening ask
	RespondentStart int64  // respondent's opening offer
	ConcessionRate float64 // fraction of the distance to each party's reservation conceded per round
	MaxRounds      int
}

// SimulateParties runs the protocol with two synthetic parties that anchor on
// opening positions and concede toward their own reservation value each round,
// until their offers cross (settlement) or the rounds run out (escalation).
// It exists to demonstrate and test convergence without two live users.
func (e *Engine) SimulateParties(ctx context.Context, d *domain.Dispute, opts SimOptions) error {
	if d.Analysis == nil {
		return domain.ErrNotAnalyzed
	}
	if opts.ConcessionRate <= 0 || opts.ConcessionRate >= 1 {
		opts.ConcessionRate = 0.5
	}
	if opts.MaxRounds <= 0 {
		opts.MaxRounds = 4
	}
	if opts.ClaimantStart == 0 {
		opts.ClaimantStart = d.ClaimAmount // anchor on the full claim
	}
	if opts.RespondentStart == 0 {
		// A lowball opening below the respondent's true ceiling.
		opts.RespondentStart = d.Analysis.ClaimantReservation * 3 / 5
	}

	if err := e.Start(d, opts.MaxRounds); err != nil {
		return err
	}

	cFloor := float64(d.Analysis.ClaimantReservation)
	rCeil := float64(d.Analysis.RespondentReservation)
	ask := float64(opts.ClaimantStart)
	pay := float64(opts.RespondentStart)

	for d.Negotiation.Status == "open" {
		round, err := e.SubmitRound(ctx, d, int64(ask), int64(pay))
		if err != nil {
			return err
		}
		if round.Outcome != domain.OutcomeContinue {
			break
		}
		// Each party concedes toward its own reservation value.
		ask -= opts.ConcessionRate * (ask - cFloor)
		pay += opts.ConcessionRate * (rCeil - pay)
	}
	return nil
}

func roundTo(x, step int64) int64 {
	if step <= 0 {
		return x
	}
	return ((x + step/2) / step) * step
}
