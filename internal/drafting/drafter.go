// Package drafting turns a settled figure into a plain-language consent
// agreement. The legal scaffolding is fixed; the model fills it in. If the
// model is unavailable the drafter falls back to a deterministic template so a
// settlement is never blocked on text generation.
package drafting

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pranav/samadhan/internal/domain"
	"github.com/pranav/samadhan/internal/llm"
)

// Drafter produces settlement agreements.
type Drafter struct {
	model llm.Provider
}

// New builds a Drafter backed by the given model provider.
func New(model llm.Provider) *Drafter { return &Drafter{model: model} }

// Draft creates a Settlement for the dispute at the agreed amount. method is
// "negotiated" or "mediator_proposal".
func (dr *Drafter) Draft(ctx context.Context, d *domain.Dispute, amount int64, method string) (*domain.Settlement, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("%w: settlement amount must be positive", domain.ErrInvalidInput)
	}
	now := time.Now().UTC()
	facts := map[string]any{
		"claimant":   d.Claimant.Name,
		"respondent": d.Respondent.Name,
		"amount":     amount,
		"category":   string(d.Category),
		"date":       now.Format("2 January 2006"),
		"case_id":    d.ID,
	}

	type draftResult struct {
		AgreementText string `json:"agreement_text"`
	}

	text := ""
	out, err := llm.CompleteJSON[draftResult](ctx, dr.model, llm.DraftRequest(facts))
	if err == nil && strings.TrimSpace(out.AgreementText) != "" {
		text = out.AgreementText
	} else {
		text = fallbackAgreement(d, amount, now)
	}

	return &domain.Settlement{
		Amount:        amount,
		Currency:      d.Currency,
		Method:        method,
		AgreementText: text,
		SettledAt:     now,
	}, nil
}

func fallbackAgreement(d *domain.Dispute, amount int64, now time.Time) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "SETTLEMENT AGREEMENT\n(Consent settlement reached through Online Dispute Resolution)\n\n")
	fmt.Fprintf(&sb, "Date: %s\nCase reference: %s\n\n", now.Format("2 January 2006"), d.ID)
	fmt.Fprintf(&sb, "PARTIES\n1. %s (\"the Claimant\"); and\n2. %s (\"the Respondent\").\n\n", d.Claimant.Name, d.Respondent.Name)
	fmt.Fprintf(&sb, "RECITALS\nA dispute arose between the parties. The parties participated in a confidential online negotiation facilitated by Samadhan and have agreed to settle without admission of liability.\n\n")
	fmt.Fprintf(&sb, "AGREED TERMS\n1. The Respondent shall pay the Claimant %s in full and final settlement of the dispute, by electronic transfer within 30 days.\n", domain.FormatINR(amount))
	fmt.Fprintf(&sb, "2. On receipt, the dispute stands fully and finally resolved and the parties mutually release each other from all related claims.\n")
	fmt.Fprintf(&sb, "3. The parties shall keep these terms confidential save as required by law.\n\n")
	fmt.Fprintf(&sb, "SIGNED:\n_______________________            _______________________\n%s (Claimant)            %s (Respondent)\n\n", d.Claimant.Name, d.Respondent.Name)
	fmt.Fprintf(&sb, "Note: This is a record of a consent settlement reached through Online Dispute Resolution. It is not legal advice.\n")
	return sb.String()
}
