// Package analysis is the Settlement Intelligence Engine. Given a dispute it
// produces a defensible settlement range by combining a qualitative read from
// the language model (how strong, how collectable, how slow) with a
// transparent micro-economic model of each party's alternative to a negotiated
// outcome. The economic layer is deterministic and fully explained, line by
// line, because a settlement figure that cannot be justified is useless to a
// case manager, a party, or a court.
package analysis

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/pranav/samadhan/internal/domain"
	"github.com/pranav/samadhan/internal/llm"
)

// Economic priors. These are deliberately explicit constants rather than magic
// numbers buried in the math, so they can be reviewed and, in production,
// sourced per-lender or per-category from historical outcome data.
const (
	// discountRate is the annual rate used to bring a future, uncertain
	// litigation recovery into present-day rupees. It stands in for the time
	// value of money and the cost of capital tied up in an unresolved dispute.
	discountRate = 0.12

	// Litigation costs are modelled as a fraction of the claim with a floor,
	// reflecting that even small matters carry a minimum cost to pursue/defend.
	claimantCostRate = 0.08
	respondentCostRate = 0.06
	minLitigationCost  = 15000.0
)

// Engine computes settlement intelligence.
type Engine struct {
	model llm.Provider
}

// New builds an Engine backed by the given model provider.
func New(model llm.Provider) *Engine { return &Engine{model: model} }

// assessment mirrors the JSON the model returns for a case assessment.
type assessment struct {
	ClaimStrength           float64  `json:"claim_strength"`
	Confidence              float64  `json:"confidence"`
	TimeToResolutionYears   float64  `json:"time_to_resolution_years"`
	RecoveryRate            float64  `json:"recovery_rate"`
	RespondentCapacityRatio float64  `json:"respondent_capacity_ratio"`
	KeyIssues               []string `json:"key_issues"`
	Summary                 string   `json:"summary"`
}

// Analyze runs the full pipeline and returns a populated CaseAnalysis.
func (e *Engine) Analyze(ctx context.Context, d *domain.Dispute) (*domain.CaseAnalysis, error) {
	if d.ClaimAmount <= 0 {
		return nil, fmt.Errorf("%w: claim amount must be positive", domain.ErrInvalidInput)
	}

	// 1) Qualitative read from the model.
	facts := disputeFacts(d)
	a, err := llm.CompleteJSON[assessment](ctx, e.model, llm.CaseAssessmentRequest(facts))
	if err != nil {
		return nil, fmt.Errorf("case assessment: %w", err)
	}
	sanitizeAssessment(&a)

	// 2) Resolve economic inputs from the qualitative read.
	claim := float64(d.ClaimAmount)
	p := a.ClaimStrength
	recovery := a.RecoveryRate
	years := a.TimeToResolutionYears
	capacity := claim * a.RespondentCapacityRatio
	claimantCost := math.Max(minLitigationCost, claimantCostRate*claim)
	respondentCost := math.Max(minLitigationCost, respondentCostRate*claim)
	pvFactor := 1.0 / math.Pow(1+discountRate, years)

	// 3) Core model.
	//
	// Claimant's BATNA: the present value of what litigation realistically
	// recovers, net of their own cost. They should accept any settlement at
	// least this good — money now beats a discounted, risky, costly recovery
	// later. This is the floor of the settlement zone.
	expectedGross := claim * p
	expectedCollectable := expectedGross * recovery
	claimantBATNA := expectedCollectable*pvFactor - claimantCost
	claimantReservation := math.Max(0, claimantBATNA)

	// Respondent's exposure: the present value of their probability-weighted
	// liability plus the cost of defending. They should pay up to this to make
	// the matter (and its risk) disappear — but never more than they can
	// actually pay. The smaller of the two is the ceiling of the zone.
	respondentExposure := expectedGross*pvFactor + respondentCost
	respondentReservation := math.Min(capacity, respondentExposure)

	// 4) ZOPA + recommendation.
	zopaExists := respondentReservation >= claimantReservation
	// Fairness weight positions the recommendation inside the zone: a stronger
	// claim is settled nearer the respondent's ceiling, a weaker one nearer the
	// claimant's floor. It centres on the midpoint and tilts with the merits.
	fairnessWeight := clamp(0.35+0.30*p, 0.0, 1.0)

	var recommended float64
	if zopaExists {
		width := respondentReservation - claimantReservation
		recommended = claimantReservation + width*fairnessWeight
	} else {
		// No rational overlap. The best available figure is the most the
		// respondent can bear; the engine flags this for a structural remedy
		// or a human neutral rather than pretending a deal exists.
		recommended = respondentReservation
	}

	res := &domain.CaseAnalysis{
		ClaimStrength: r2(p),
		KeyIssues:     a.KeyIssues,
		Summary:       a.Summary,
		Confidence:    r2(a.Confidence),

		TimeToResolutionYears:    years,
		DiscountRate:             discountRate,
		RecoveryRate:             recovery,
		RespondentCapacity:       rupees(capacity),
		ClaimantLitigationCost:   rupees(claimantCost),
		RespondentLitigationCost: rupees(respondentCost),

		ClaimantReservation:   rupees(claimantReservation),
		RespondentReservation: rupees(respondentReservation),
		ZOPAExists:            zopaExists,
		RecommendedSettlement: roundTo(rupees(recommended), 1000),
		FairnessWeight:        r2(fairnessWeight),

		Provider:    e.model.Name(),
		GeneratedAt: time.Now().UTC(),
	}
	if zopaExists {
		res.ZOPALow = res.ClaimantReservation
		res.ZOPAHigh = res.RespondentReservation
		res.ZOPAWidth = res.ZOPAHigh - res.ZOPALow
	}

	res.Rationale = buildRationale(d, &a, res, expectedGross, expectedCollectable, pvFactor,
		claimantCost, respondentCost, respondentExposure, capacity)
	res.Recommendations = buildRecommendations(res, respondentExposure, capacity)

	return res, nil
}

// disputeFacts shapes a dispute into the structured context the prompt and the
// offline provider both read.
func disputeFacts(d *domain.Dispute) map[string]any {
	docTypes := make([]string, 0, len(d.Documents))
	for _, doc := range d.Documents {
		docTypes = append(docTypes, doc.Type)
	}
	return map[string]any{
		"category":            string(d.Category),
		"title":               d.Title,
		"claim_amount":        d.ClaimAmount,
		"currency":            d.Currency,
		"narrative":           d.Narrative,
		"respondent_response": d.RespondentResponse,
		"document_types":      docTypes,
	}
}

// sanitizeAssessment clamps model outputs into valid ranges and fills sane
// defaults so a malformed reply can never produce nonsensical economics.
func sanitizeAssessment(a *assessment) {
	a.ClaimStrength = clamp(a.ClaimStrength, 0.02, 0.98)
	a.Confidence = clamp(a.Confidence, 0.0, 1.0)
	a.RecoveryRate = clamp(a.RecoveryRate, 0.05, 1.0)
	a.RespondentCapacityRatio = clamp(a.RespondentCapacityRatio, 0.05, 1.5)
	if a.TimeToResolutionYears <= 0 || a.TimeToResolutionYears > 10 {
		a.TimeToResolutionYears = 1.8
	}
	if len(a.KeyIssues) == 0 {
		a.KeyIssues = []string{"Liability", "Quantum of the claim"}
	}
	if a.Summary == "" {
		a.Summary = "Settlement range derived from each side's realistic alternative to a negotiated outcome."
	}
}

func buildRationale(d *domain.Dispute, a *assessment, res *domain.CaseAnalysis,
	expectedGross, expectedCollectable, pvFactor, claimantCost, respondentCost,
	respondentExposure, capacity float64) []string {

	lines := []string{
		fmt.Sprintf("Claimed amount: %s.", domain.FormatINR(d.ClaimAmount)),
		fmt.Sprintf("Modelled merits probability (claimant prevails): %.0f%% (model confidence %.0f%%).", a.ClaimStrength*100, a.Confidence*100),
		fmt.Sprintf("Probability-weighted award if adjudicated: %s × %.0f%% = %s.", domain.FormatINR(d.ClaimAmount), a.ClaimStrength*100, domain.FormatINR(rupees(expectedGross))),
		fmt.Sprintf("Collectability of an award: %.0f%% → expected collectable %s.", a.RecoveryRate*100, domain.FormatINR(rupees(expectedCollectable))),
		fmt.Sprintf("Time to resolution: %.1f years; at a %.0f%% discount rate the present-value factor is %.2f.", a.TimeToResolutionYears, discountRate*100, pvFactor),
		fmt.Sprintf("Claimant's floor (BATNA): present value of collectable recovery %s, less own legal cost %s = %s. The claimant should accept any settlement at or above this.",
			domain.FormatINR(rupees(expectedCollectable*pvFactor)), domain.FormatINR(rupees(claimantCost)), domain.FormatINR(res.ClaimantReservation)),
	}
	if respondentExposure <= capacity {
		lines = append(lines, fmt.Sprintf("Respondent's ceiling: present value of probability-weighted liability %s, plus own legal cost %s = %s. The respondent should pay up to this to avoid the risk and delay.",
			domain.FormatINR(rupees(expectedGross*pvFactor)), domain.FormatINR(rupees(respondentCost)), domain.FormatINR(res.RespondentReservation)))
	} else {
		lines = append(lines, fmt.Sprintf("Respondent's ceiling: their risk-based exposure is %s, but ability to pay caps it at %s. Capacity is the binding constraint.",
			domain.FormatINR(rupees(respondentExposure)), domain.FormatINR(res.RespondentReservation)))
	}
	if res.ZOPAExists {
		lines = append(lines,
			fmt.Sprintf("Zone of Possible Agreement: %s – %s (width %s).", domain.FormatINR(res.ZOPALow), domain.FormatINR(res.ZOPAHigh), domain.FormatINR(res.ZOPAWidth)),
			fmt.Sprintf("Recommended settlement: %s (fairness weight %.2f within the zone, tilted by the merits).", domain.FormatINR(res.RecommendedSettlement), res.FairnessWeight))
	} else {
		lines = append(lines,
			fmt.Sprintf("No overlap: the claimant's floor (%s) exceeds the respondent's ceiling (%s). A straight cash settlement is not rational for both sides as modelled.", domain.FormatINR(res.ClaimantReservation), domain.FormatINR(res.RespondentReservation)))
	}
	return lines
}

func buildRecommendations(res *domain.CaseAnalysis, respondentExposure, capacity float64) []string {
	var recs []string
	if !res.ZOPAExists {
		if respondentExposure > capacity {
			recs = append(recs, "Capacity is the binding constraint: converting the settlement into an instalment plan (e.g. 9–18 months) raises what the respondent can effectively pay and can open a Zone of Possible Agreement that does not exist for a single lump sum.")
		}
		recs = append(recs, "If no structural remedy creates an overlap, escalate to a human neutral for evaluative mediation.")
	} else {
		if res.ZOPAWidth < res.RecommendedSettlement/10 {
			recs = append(recs, "The agreement zone is narrow, so positions are already close — a settlement is likely within one or two bidding rounds.")
		} else {
			recs = append(recs, "The agreement zone is wide — the double-blind-bid protocol should be used so neither party anchors the other and value is shared fairly.")
		}
	}
	recs = append(recs, "This is decision-support derived from a transparent model, not legal advice or an adjudication of the merits.")
	return recs
}

// ---- numeric helpers ------------------------------------------------------

func clamp(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

func rupees(x float64) int64 { return int64(math.Round(x)) }

func r2(x float64) float64 { return math.Round(x*100) / 100 }

func roundTo(x, step int64) int64 {
	if step <= 0 {
		return x
	}
	return ((x + step/2) / step) * step
}
