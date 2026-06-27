// Package domain holds the core business entities for Samadhan. It has no
// dependencies on transport, storage, or the LLM layer so that the rules of
// dispute resolution can be reasoned about (and tested) in isolation.
package domain

import "time"

// Category buckets a dispute into a resolution archetype. The category drives
// the economic priors used by the settlement-intelligence engine (expected
// time-to-resolution, collectability, typical defences and so on).
type Category string

const (
	CategoryLoanDefault       Category = "loan_default"
	CategoryChequeBounce      Category = "cheque_bounce"
	CategoryEcommerce         Category = "ecommerce"
	CategoryRentTenancy       Category = "rent_tenancy"
	CategoryServiceDeficiency Category = "service_deficiency"
	CategoryGeneric           Category = "generic"
)

// Valid reports whether c is a category the engine knows how to price.
func (c Category) Valid() bool {
	switch c {
	case CategoryLoanDefault, CategoryChequeBounce, CategoryEcommerce,
		CategoryRentTenancy, CategoryServiceDeficiency, CategoryGeneric:
		return true
	}
	return false
}

// DisputeStatus is the lifecycle stage of a dispute.
type DisputeStatus string

const (
	StatusIntake      DisputeStatus = "intake"      // created, not yet analysed
	StatusAnalyzed    DisputeStatus = "analyzed"    // settlement intelligence computed
	StatusNegotiating DisputeStatus = "negotiating" // double-blind bidding underway
	StatusSettled     DisputeStatus = "settled"     // agreement reached
	StatusEscalated   DisputeStatus = "escalated"   // handed to a human neutral
)

// Party is a participant in the dispute.
type Party struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// Document is a piece of evidence attached to a dispute. In production the
// Summary would be produced by an OCR + extraction pipeline; here it is the
// short text the intake layer works with.
type Document struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // loan_agreement, invoice, cheque, communication, ...
	Summary string `json:"summary"`
}

// Dispute is the aggregate root. Everything the system knows about a case
// hangs off this struct.
type Dispute struct {
	ID                 string        `json:"id"`
	Category           Category      `json:"category"`
	Title              string        `json:"title"`
	Claimant           Party         `json:"claimant"`
	Respondent         Party         `json:"respondent"`
	ClaimAmount        int64         `json:"claim_amount"` // whole rupees
	Currency           string        `json:"currency"`
	Narrative          string        `json:"narrative"`                     // claimant's account of the facts
	RespondentResponse string        `json:"respondent_response,omitempty"` // optional reply
	Documents          []Document    `json:"documents,omitempty"`
	Status             DisputeStatus `json:"status"`
	Analysis           *CaseAnalysis `json:"analysis,omitempty"`
	Negotiation        *Negotiation  `json:"negotiation,omitempty"`
	Settlement         *Settlement   `json:"settlement,omitempty"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}
