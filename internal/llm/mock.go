package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
)

// MockProvider produces deterministic, schema-correct responses with no
// network access. It is not a toy stub: it reads the same structured facts the
// real model receives and applies transparent heuristics, so the end-to-end
// flow — assessment, nudges, drafting — is coherent and demoable offline and
// in CI. When ANTHROPIC_API_KEY is set the real model is used instead.
type MockProvider struct{}

// NewMock builds the offline provider.
func NewMock() *MockProvider { return &MockProvider{} }

func (m *MockProvider) Name() string { return "offline-model" }

// Complete routes to a handler based on the task tag and returns JSON text.
func (m *MockProvider) Complete(_ context.Context, req Request) (Response, error) {
	var payload any
	switch req.Task {
	case TaskCaseAssessment:
		payload = m.assess(req.Facts)
	case TaskNudge:
		payload = m.nudge(req.Facts)
	case TaskDraft:
		payload = m.draft(req.Facts)
	default:
		payload = map[string]any{"note": "no offline handler for task"}
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return Response{}, err
	}
	return Response{Text: string(b), Provider: m.Name()}, nil
}

// ---- task handlers --------------------------------------------------------

type assessOut struct {
	ClaimStrength          float64  `json:"claim_strength"`
	Confidence             float64  `json:"confidence"`
	TimeToResolutionYears  float64  `json:"time_to_resolution_years"`
	RecoveryRate           float64  `json:"recovery_rate"`
	RespondentCapacityRatio float64 `json:"respondent_capacity_ratio"`
	KeyIssues              []string `json:"key_issues"`
	Summary                string   `json:"summary"`
}

func (m *MockProvider) assess(f map[string]any) assessOut {
	cat := getString(f, "category", "generic")
	claim := getFloat(f, "claim_amount", 0)
	narrative := strings.ToLower(getString(f, "narrative", ""))
	respResp := strings.ToLower(getString(f, "respondent_response", ""))
	docTypes := getStrings(f, "document_types")

	pr := jitter(narrative + respResp) // stable per-case variation in [-1,1]

	p := categoryBaseStrength(cat)
	for _, d := range docTypes {
		switch strings.ToLower(d) {
		case "loan_agreement", "contract":
			p += 0.06
		case "cheque":
			p += 0.06
		case "acknowledgement", "acknowledgment", "statement_of_account":
			p += 0.05
		case "invoice", "demand_notice":
			p += 0.04
		case "communication":
			p += 0.02
		}
	}
	if respResp != "" {
		p -= 0.05 // actively contested
	}
	for _, kw := range []string{"forged", "fraud", "never received", "denies", "no agreement", "defective", "coerc", "dispute the"} {
		if strings.Contains(narrative, kw) || strings.Contains(respResp, kw) {
			p -= 0.05
			break
		}
	}
	p += 0.03 * pr
	p = clamp(p, 0.05, 0.95)

	conf := 0.4 + 0.1*float64(len(docTypes))
	conf = clamp(conf, 0.35, 0.9)

	// Capacity to pay. Normally driven by the category prior, but an explicit
	// plea of financial distress in the respondent's reply sharply lowers what
	// they can realistically pay as a lump sum. This is what lets the engine
	// surface its capacity-constrained branch (instalment remedy / escalation)
	// rather than assuming every respondent can fund a cash settlement.
	capacityRatio := clamp(categoryCapacity(cat)+0.05*pr, 0.2, 1.0)
	if mentionsDistress(respResp) || mentionsDistress(narrative) {
		capacityRatio = 0.20
	}

	return assessOut{
		ClaimStrength:           round2(p),
		Confidence:              round2(conf),
		TimeToResolutionYears:   categoryYears(cat),
		RecoveryRate:            categoryRecovery(cat),
		RespondentCapacityRatio: round2(capacityRatio),
		KeyIssues:               categoryIssues(cat),
		Summary:                 assessmentSummary(cat, p, claim),
	}
}

// mentionsDistress reports whether text contains an explicit inability-to-pay
// signal. Kept deliberately narrow so ordinary denials don't trip it.
func mentionsDistress(text string) bool {
	for _, kw := range []string{
		"cannot pay", "can't pay", "cant pay", "unable to pay", "no funds",
		"insolven", "financial hardship", "financial distress", "lost my job",
		"shut down", "shutting down", "bankrupt", "no capacity to pay",
	} {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}

type nudgeOut struct {
	Message        string `json:"message"`
	SuggestedOffer int64  `json:"suggested_offer"`
}

func (m *MockProvider) nudge(f map[string]any) nudgeOut {
	party := getString(f, "party", "claimant")
	alt := getInt(f, "their_alternative", 0)
	current := getInt(f, "current_offer", 0)
	rec := getInt(f, "recommended", 0)

	// Move 40% of the way from the current offer toward the fair figure.
	var suggested int64
	if party == "respondent" {
		suggested = current + (rec-current)*2/5
		if suggested > rec {
			suggested = rec
		}
	} else {
		suggested = current - (current-rec)*2/5
		if suggested < rec {
			suggested = rec
		}
	}
	suggested = roundTo(suggested, 1000)

	var msg string
	if party == "respondent" {
		msg = fmt.Sprintf(
			"If this matter is not settled, the modelled value of your exposure if it is adjudicated is about %s, before counting your own legal costs, the delay, and the uncertainty of the outcome. Settling now near the fair range removes that risk and closes the matter on terms you control. A revised offer around %s would be a constructive move.",
			inr(alt), inr(suggested))
	} else {
		msg = fmt.Sprintf(
			"If this matter is not settled, the modelled present value of what litigation realistically recovers for you is about %s, after accounting for the years it takes, your own costs, and the risk of recovery. A settlement at or above that figure is rational and immediate. Bringing your ask toward %s would help close the gap.",
			inr(alt), inr(suggested))
	}
	return nudgeOut{Message: msg, SuggestedOffer: suggested}
}

type draftOut struct {
	AgreementText string `json:"agreement_text"`
}

func (m *MockProvider) draft(f map[string]any) draftOut {
	claimant := getString(f, "claimant", "Claimant")
	respondent := getString(f, "respondent", "Respondent")
	amount := getInt(f, "amount", 0)
	cat := getString(f, "category", "generic")
	date := getString(f, "date", "")
	caseID := getString(f, "case_id", "")

	var sb strings.Builder
	fmt.Fprintf(&sb, "SETTLEMENT AGREEMENT\n")
	fmt.Fprintf(&sb, "(Consent settlement reached through Online Dispute Resolution)\n\n")
	fmt.Fprintf(&sb, "Date: %s\nCase reference: %s\n\n", date, caseID)
	fmt.Fprintf(&sb, "PARTIES\n")
	fmt.Fprintf(&sb, "1. %s (\"the Claimant\"); and\n", claimant)
	fmt.Fprintf(&sb, "2. %s (\"the Respondent\").\n\n", respondent)
	fmt.Fprintf(&sb, "RECITALS\n")
	fmt.Fprintf(&sb, "A. A dispute arose between the parties in connection with a %s matter.\n", humanizeCategory(cat))
	fmt.Fprintf(&sb, "B. The parties participated in a confidential online negotiation facilitated by Samadhan and have agreed to resolve the dispute on the terms below, without admission of liability by either party.\n\n")
	fmt.Fprintf(&sb, "AGREED TERMS\n")
	fmt.Fprintf(&sb, "1. Settlement sum. The Respondent shall pay the Claimant %s (the \"Settlement Sum\") in full and final settlement of the dispute.\n", inr(amount))
	fmt.Fprintf(&sb, "2. Manner of payment. The Settlement Sum shall be paid by electronic transfer within 30 days of the date of this Agreement, unless the parties agree an instalment schedule in writing.\n")
	fmt.Fprintf(&sb, "3. Full and final settlement. On receipt of the Settlement Sum, the dispute stands fully and finally resolved and neither party shall have any further claim against the other arising from the subject matter.\n")
	fmt.Fprintf(&sb, "4. Mutual release. Each party releases the other from all claims, demands and causes of action relating to the dispute, whether known or unknown as at the date of this Agreement.\n")
	fmt.Fprintf(&sb, "5. Confidentiality. The parties shall keep the terms of this Agreement confidential, save as required by law or to enforce its terms.\n")
	fmt.Fprintf(&sb, "6. Enforceability. This Agreement records a settlement and may be enforced in accordance with applicable law governing settlements reached through alternative dispute resolution.\n\n")
	fmt.Fprintf(&sb, "SIGNED for and on behalf of the parties:\n\n")
	fmt.Fprintf(&sb, "_______________________            _______________________\n")
	fmt.Fprintf(&sb, "%s (Claimant)            %s (Respondent)\n\n", claimant, respondent)
	fmt.Fprintf(&sb, "Note: This document is a record of a consent settlement reached through Online Dispute Resolution. It is not legal advice. Parties should retain a copy for their records.\n")

	return draftOut{AgreementText: sb.String()}
}

// ---- category priors ------------------------------------------------------

func categoryBaseStrength(c string) float64 {
	switch c {
	case "loan_default":
		return 0.72
	case "cheque_bounce":
		return 0.80
	case "ecommerce":
		return 0.58
	case "rent_tenancy":
		return 0.60
	case "service_deficiency":
		return 0.55
	default:
		return 0.55
	}
}

func categoryYears(c string) float64 {
	switch c {
	case "loan_default":
		return 2.0
	case "cheque_bounce":
		return 1.8
	case "ecommerce":
		return 1.2
	case "rent_tenancy":
		return 1.5
	case "service_deficiency":
		return 1.3
	default:
		return 1.6
	}
}

func categoryRecovery(c string) float64 {
	switch c {
	case "loan_default":
		return 0.55
	case "cheque_bounce":
		return 0.60
	case "ecommerce":
		return 0.70
	case "rent_tenancy":
		return 0.65
	case "service_deficiency":
		return 0.70
	default:
		return 0.65
	}
}

func categoryCapacity(c string) float64 {
	switch c {
	case "loan_default":
		return 0.60
	case "cheque_bounce":
		return 0.65
	case "ecommerce":
		return 0.90
	case "rent_tenancy":
		return 0.80
	case "service_deficiency":
		return 0.95
	default:
		return 0.80
	}
}

func categoryIssues(c string) []string {
	switch c {
	case "loan_default":
		return []string{"Quantum of outstanding (principal vs interest and charges)", "Validity and service of the default notice", "Borrower's repayment capacity"}
	case "cheque_bounce":
		return []string{"Existence of a legally enforceable debt", "Service of the statutory demand notice", "Rebuttal of the presumption under s.139 NI Act"}
	case "ecommerce":
		return []string{"Whether goods/services conformed to the contract", "Applicability of the refund/return policy", "Quantum of the loss claimed"}
	case "rent_tenancy":
		return []string{"Computation of arrears", "Condition of premises and deposit deductions", "Validity of the termination"}
	case "service_deficiency":
		return []string{"Standard of service promised versus delivered", "Causation of the loss", "Quantum of compensation"}
	default:
		return []string{"Liability", "Quantum of the claim", "Documentary support"}
	}
}

func assessmentSummary(c string, p, claim float64) string {
	band := "a moderate"
	switch {
	case p >= 0.75:
		band = "a strong"
	case p < 0.5:
		band = "a contestable"
	}
	return fmt.Sprintf(
		"This is %s claim in a %s matter, with a modelled merits probability of about %.0f%%. The claimed amount is %s. The figure below is a settlement range derived from each side's realistic alternative to a negotiated outcome; it is decision-support for the parties, not an adjudication.",
		band, humanizeCategory(c), p*100, inr(int64(claim)))
}

func humanizeCategory(c string) string {
	switch c {
	case "loan_default":
		return "loan default"
	case "cheque_bounce":
		return "dishonoured cheque"
	case "ecommerce":
		return "e-commerce"
	case "rent_tenancy":
		return "rent / tenancy"
	case "service_deficiency":
		return "service deficiency"
	default:
		return "commercial"
	}
}

// ---- small helpers --------------------------------------------------------

func getString(f map[string]any, k, def string) string {
	if v, ok := f[k]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func getStrings(f map[string]any, k string) []string {
	if v, ok := f[k]; ok {
		if s, ok := v.([]string); ok {
			return s
		}
	}
	return nil
}

func getFloat(f map[string]any, k string, def float64) float64 {
	if v, ok := f[k]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int64:
			return float64(n)
		case int:
			return float64(n)
		}
	}
	return def
}

func getInt(f map[string]any, k string, def int64) int64 {
	if v, ok := f[k]; ok {
		switch n := v.(type) {
		case int64:
			return n
		case int:
			return int64(n)
		case float64:
			return int64(n)
		}
	}
	return def
}

func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func round2(x float64) float64 { return math.Round(x*100) / 100 }

func roundTo(x, step int64) int64 {
	if step <= 0 {
		return x
	}
	return ((x + step/2) / step) * step
}

// jitter maps a string to a stable value in [-1, 1] for per-case variation.
func jitter(s string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return (float64(h.Sum32()%1000)/1000.0)*2 - 1
}

// inr formats whole rupees with Indian digit grouping (local copy to keep this
// package free of a domain import).
func inr(rupees int64) string {
	neg := rupees < 0
	if neg {
		rupees = -rupees
	}
	s := fmt.Sprintf("%d", rupees)
	n := len(s)
	if n <= 3 {
		if neg {
			return "₹-" + s
		}
		return "₹" + s
	}
	out := s[n-3:]
	s = s[:n-3]
	for len(s) > 2 {
		out = s[len(s)-2:] + "," + out
		s = s[:len(s)-2]
	}
	out = s + "," + out
	if neg {
		return "₹-" + out
	}
	return "₹" + out
}
