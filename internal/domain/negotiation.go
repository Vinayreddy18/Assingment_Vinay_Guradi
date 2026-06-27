package domain

import "time"

// OfferParty identifies who made an offer.
type OfferParty string

const (
	PartyClaimant   OfferParty = "claimant"
	PartyRespondent OfferParty = "respondent"
)

// RoundOutcome is the result of processing one round of the protocol.
type RoundOutcome string

const (
	OutcomeContinue  RoundOutcome = "continue"  // gap remains, keep bidding
	OutcomeSettled   RoundOutcome = "settled"   // offers crossed, deal done
	OutcomeEscalated RoundOutcome = "escalated" // exhausted rounds, refer to neutral
)

// Nudge is a neutral, party-specific message generated to encourage movement.
// Crucially it is grounded in *that party's own* alternative to settling
// (their BATNA/exposure), never in the other side's confidential number.
type Nudge struct {
	Party          OfferParty `json:"party"`
	Message        string     `json:"message"`
	SuggestedOffer int64      `json:"suggested_offer"`
	Round          int        `json:"round"`
}

// Round captures one synchronous step of the double-blind-bid protocol. Both
// parties submit a confidential figure; the engine reveals only the outcome,
// not the opposing number.
type Round struct {
	Number        int          `json:"number"`
	ClaimantAsk   int64        `json:"claimant_ask"`
	RespondentPay int64        `json:"respondent_pay"`
	Gap           int64        `json:"gap"` // ask - pay; <= 0 means overlap
	Outcome       RoundOutcome `json:"outcome"`
	SettledAmount int64        `json:"settled_amount,omitempty"`
	Nudges        []Nudge      `json:"nudges,omitempty"`
	Note          string       `json:"note"`
	At            time.Time    `json:"at"`
}

// Negotiation is the running state of the bidding process for a dispute.
type Negotiation struct {
	Protocol      string  `json:"protocol"` // "double_blind_bid"
	MaxRounds     int     `json:"max_rounds"`
	CurrentRound  int     `json:"current_round"`
	Rounds        []Round `json:"rounds"`
	Status        string  `json:"status"` // open | settled | escalated
	SettledAmount int64   `json:"settled_amount,omitempty"`
}

// Settlement is the final, enforceable outcome plus the drafted agreement.
type Settlement struct {
	Amount        int64     `json:"amount"`
	Currency      string    `json:"currency"`
	Method        string    `json:"method"` // negotiated | mediator_proposal
	AgreementText string    `json:"agreement_text"`
	SettledAt     time.Time `json:"settled_at"`
}
