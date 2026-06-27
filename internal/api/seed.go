package api

import (
	"log/slog"

	"github.com/pranav/samadhan/internal/domain"
)

// Seed populates the store with a small set of realistic disputes so the UI and
// the demo have something to work with on a fresh boot. Cases are left at the
// intake stage so the operator can drive each transition (analyse, negotiate,
// settle) live. Seeding is best-effort: a failure is logged, not fatal.
func Seed(svc *Service, log *slog.Logger) {
	for _, in := range seedDisputes() {
		if _, err := svc.CreateDispute(in); err != nil {
			log.Warn("seed dispute failed", "title", in.Title, "err", err)
		}
	}
	log.Info("seeded demo disputes", "count", len(seedDisputes()))
}

func seedDisputes() []CreateDisputeInput {
	return []CreateDisputeInput{
		// 1) Cheque bounce — a strong, well-documented claim with a clear ZOPA.
		// The headline "analyse then negotiate to a deal" demo case.
		{
			Category:    string(domain.CategoryChequeBounce),
			Title:       "Dishonoured cheque — Meridian Supplies v. Kohli Traders",
			Claimant:    domain.Party{Name: "Meridian Supplies Pvt Ltd", Email: "accounts@meridiansupplies.example"},
			Respondent:  domain.Party{Name: "Kohli Traders", Email: "rk@kohlitraders.example"},
			ClaimAmount: 250000,
			Currency:    "INR",
			Narrative:   "A cheque issued by the respondent towards goods supplied was dishonoured for insufficient funds. A statutory demand notice was served and 15 days have elapsed without payment.",
			Documents: []domain.Document{
				{Name: "Cheque No. 004821", Type: "cheque", Summary: "Cheque for ₹2,50,000 returned unpaid, marked 'funds insufficient'."},
				{Name: "Demand notice", Type: "demand_notice", Summary: "Statutory notice under s.138 NI Act served by registered post; delivery acknowledged."},
				{Name: "Tax invoice", Type: "invoice", Summary: "Invoice for the goods supplied matching the cheque amount."},
			},
		},
		// 2) Loan default — clean overlap, used for the "accept the neutral
		// number" fast path where both sides simply take the recommendation.
		{
			Category:    string(domain.CategoryLoanDefault),
			Title:       "Personal loan default — Arclight Finance v. S. Nair",
			Claimant:    domain.Party{Name: "Arclight Finance Ltd", Email: "recovery@arclightfin.example"},
			Respondent:  domain.Party{Name: "Sunil Nair", Email: "sunil.nair@example"},
			ClaimAmount: 500000,
			Currency:    "INR",
			Narrative:   "The borrower stopped servicing an unsecured personal loan after 14 EMIs. The outstanding principal and accrued charges are claimed. The borrower remains employed and willing to discuss a one-time settlement.",
			Documents: []domain.Document{
				{Name: "Loan agreement", Type: "loan_agreement", Summary: "Executed agreement setting out principal, tenure and default terms."},
				{Name: "Statement of account", Type: "statement_of_account", Summary: "Ledger showing EMIs paid, date of default and outstanding balance."},
				{Name: "Default notice", Type: "demand_notice", Summary: "Notice of default and recall served on the borrower."},
			},
		},
		// 3) Loan default with an explicit plea of inability to pay — capacity
		// becomes the binding constraint, so no lump-sum ZOPA exists and the
		// engine recommends an instalment remedy / escalation. Shows judgement.
		{
			Category:    string(domain.CategoryLoanDefault),
			Title:       "Business loan default — Pinnacle Capital v. Verma Textiles",
			Claimant:    domain.Party{Name: "Pinnacle Capital", Email: "legal@pinnaclecap.example"},
			Respondent:  domain.Party{Name: "Verma Textiles", Email: "ops@vermatextiles.example"},
			ClaimAmount: 1200000,
			Currency:    "INR",
			Narrative:   "A working-capital loan fell into default after the respondent's principal buyer cancelled orders. The full outstanding is claimed along with default interest.",
			RespondentResponse: "We do not dispute the loan, but the business has effectively shut down and we cannot pay this as a lump sum. We are in acute financial hardship and can only manage small monthly instalments.",
			Documents: []domain.Document{
				{Name: "Facility agreement", Type: "loan_agreement", Summary: "Working-capital facility with repayment schedule and default interest clause."},
				{Name: "Account statement", Type: "statement_of_account", Summary: "Drawdowns and repayments to date with the defaulted balance."},
			},
		},
	}
}
