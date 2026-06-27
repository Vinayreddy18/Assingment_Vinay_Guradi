package analysis

import (
	"context"
	"testing"

	"github.com/pranav/samadhan/internal/domain"
	"github.com/pranav/samadhan/internal/llm"
)

// newEngine builds an engine backed by the deterministic offline provider so
// tests are hermetic and need no network or API key.
func newEngine() *Engine { return New(llm.NewMock()) }

func chequeDispute(claim int64) *domain.Dispute {
	return &domain.Dispute{
		ID:          "T-CHEQUE",
		Category:    domain.CategoryChequeBounce,
		Title:       "test cheque",
		Claimant:    domain.Party{Name: "C"},
		Respondent:  domain.Party{Name: "R"},
		ClaimAmount: claim,
		Currency:    "INR",
		Narrative:   "cheque dishonoured for insufficient funds; demand notice served",
		Documents: []domain.Document{
			{Name: "chq", Type: "cheque", Summary: "returned unpaid"},
			{Name: "dn", Type: "demand_notice", Summary: "served"},
		},
	}
}

func TestAnalyze_ZOPAInvariants(t *testing.T) {
	e := newEngine()
	d := chequeDispute(250000)

	a, err := e.Analyze(context.Background(), d)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}

	// Probabilities and rates must be within valid bounds.
	if a.ClaimStrength < 0 || a.ClaimStrength > 1 {
		t.Errorf("claim strength out of range: %v", a.ClaimStrength)
	}
	if a.RecoveryRate <= 0 || a.RecoveryRate > 1 {
		t.Errorf("recovery rate out of range: %v", a.RecoveryRate)
	}
	// A floor and ceiling must always be produced.
	if a.ClaimantReservation < 0 {
		t.Errorf("negative claimant reservation: %d", a.ClaimantReservation)
	}
	// For a strong, documented cheque claim a ZOPA should exist.
	if !a.ZOPAExists {
		t.Fatalf("expected a ZOPA for a strong cheque claim, got none (floor=%d ceiling=%d)",
			a.ClaimantReservation, a.RespondentReservation)
	}
	// ZOPA bounds must be internally consistent.
	if a.ZOPALow != a.ClaimantReservation || a.ZOPAHigh != a.RespondentReservation {
		t.Errorf("ZOPA bounds mismatch: low=%d floor=%d high=%d ceiling=%d",
			a.ZOPALow, a.ClaimantReservation, a.ZOPAHigh, a.RespondentReservation)
	}
	if a.ZOPAWidth != a.ZOPAHigh-a.ZOPALow {
		t.Errorf("width mismatch: %d != %d", a.ZOPAWidth, a.ZOPAHigh-a.ZOPALow)
	}
	// The recommendation must sit inside the zone.
	if a.RecommendedSettlement < a.ZOPALow || a.RecommendedSettlement > a.ZOPAHigh {
		t.Errorf("recommendation %d outside zone [%d, %d]",
			a.RecommendedSettlement, a.ZOPALow, a.ZOPAHigh)
	}
	// The recommendation is rounded to the nearest ₹1,000.
	if a.RecommendedSettlement%1000 != 0 {
		t.Errorf("recommendation not rounded to 1000: %d", a.RecommendedSettlement)
	}
	// Explainability must be populated — this product cannot ship a bare number.
	if len(a.Rationale) == 0 {
		t.Error("expected a non-empty rationale")
	}
}

func TestAnalyze_NoZOPAWhenCapacityBinds(t *testing.T) {
	e := newEngine()
	d := &domain.Dispute{
		ID:          "T-DISTRESS",
		Category:    domain.CategoryLoanDefault,
		Title:       "distressed loan",
		Claimant:    domain.Party{Name: "Lender"},
		Respondent:  domain.Party{Name: "Borrower"},
		ClaimAmount: 1200000,
		Currency:    "INR",
		Narrative:   "working capital loan in default",
		RespondentResponse: "we do not dispute the debt but the business has shut down and we cannot pay a lump sum; acute financial hardship",
		Documents: []domain.Document{
			{Name: "fa", Type: "loan_agreement", Summary: "facility"},
		},
	}

	a, err := e.Analyze(context.Background(), d)
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if a.ZOPAExists {
		t.Fatalf("expected no ZOPA when capacity binds, got floor=%d ceiling=%d",
			a.ClaimantReservation, a.RespondentReservation)
	}
	// The ceiling should be the (reduced) capacity, below the floor.
	if a.RespondentReservation >= a.ClaimantReservation {
		t.Errorf("expected ceiling below floor: ceiling=%d floor=%d",
			a.RespondentReservation, a.ClaimantReservation)
	}
	// An instalment remedy must be recommended in this branch.
	foundInstalment := false
	for _, r := range a.Recommendations {
		if containsFold(r, "instalment") {
			foundInstalment = true
		}
	}
	if !foundInstalment {
		t.Errorf("expected an instalment recommendation, got %v", a.Recommendations)
	}
}

func TestAnalyze_StrongerClaimSettlesHigher(t *testing.T) {
	e := newEngine()

	// Same claim and category; the only difference is documentary support,
	// which raises modelled merits. A stronger claim should not settle lower.
	weak := chequeDispute(250000)
	weak.Documents = nil // fewer documents => lower claim strength
	strong := chequeDispute(250000)

	aw, err := e.Analyze(context.Background(), weak)
	if err != nil {
		t.Fatalf("weak analyze: %v", err)
	}
	as, err := e.Analyze(context.Background(), strong)
	if err != nil {
		t.Fatalf("strong analyze: %v", err)
	}

	if as.ClaimStrength < aw.ClaimStrength {
		t.Fatalf("expected stronger documented claim to have >= strength: strong=%.2f weak=%.2f",
			as.ClaimStrength, aw.ClaimStrength)
	}
	if as.RecommendedSettlement < aw.RecommendedSettlement {
		t.Errorf("stronger claim settled lower: strong=%d weak=%d",
			as.RecommendedSettlement, aw.RecommendedSettlement)
	}
}

func TestAnalyze_Deterministic(t *testing.T) {
	e := newEngine()
	d := chequeDispute(250000)

	a1, err := e.Analyze(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := e.Analyze(context.Background(), chequeDispute(250000))
	if err != nil {
		t.Fatal(err)
	}
	if a1.RecommendedSettlement != a2.RecommendedSettlement ||
		a1.ClaimantReservation != a2.ClaimantReservation ||
		a1.RespondentReservation != a2.RespondentReservation {
		t.Errorf("offline analysis is not deterministic: %+v vs %+v", a1, a2)
	}
}

func TestAnalyze_RejectsNonPositiveClaim(t *testing.T) {
	e := newEngine()
	d := chequeDispute(0)
	if _, err := e.Analyze(context.Background(), d); err == nil {
		t.Error("expected an error for a non-positive claim amount")
	}
}

// containsFold is a tiny case-insensitive substring check kept local to avoid a
// strings import in the test for one use.
func containsFold(haystack, needle string) bool {
	h := []rune(haystack)
	n := []rune(needle)
	if len(n) == 0 {
		return true
	}
	lower := func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 32
		}
		return r
	}
	for i := 0; i+len(n) <= len(h); i++ {
		match := true
		for j := 0; j < len(n); j++ {
			if lower(h[i+j]) != lower(n[j]) {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
