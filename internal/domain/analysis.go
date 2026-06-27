package domain

import "time"

// CaseAnalysis is the output of the Settlement Intelligence Engine. It is a
// hybrid object: the qualitative fields are assessed by the LLM from the case
// facts and documents, while the economic fields are derived by a transparent,
// auditable model. Keeping the two layers separate is deliberate — a legal
// product must be able to show *why* it suggested a number, line by line, and
// a black-box prediction cannot survive a regulator or a court.
type CaseAnalysis struct {
	// ---- Qualitative layer (LLM-assessed) -------------------------------
	// ClaimStrength is the modelled probability (0..1) that the claimant would
	// prevail on the merits if the matter were adjudicated.
	ClaimStrength float64  `json:"claim_strength"`
	KeyIssues     []string `json:"key_issues"`
	Summary       string   `json:"summary"`
	Confidence    float64  `json:"confidence"` // model's confidence in its own read of the facts

	// ---- Economic inputs (resolved priors, adjustable per case) ---------
	TimeToResolutionYears    float64 `json:"time_to_resolution_years"`
	DiscountRate             float64 `json:"discount_rate"`
	RecoveryRate             float64 `json:"recovery_rate"` // collectability if claimant wins
	RespondentCapacity       int64   `json:"respondent_capacity"`
	ClaimantLitigationCost   int64   `json:"claimant_litigation_cost"`
	RespondentLitigationCost int64   `json:"respondent_litigation_cost"`

	// ---- Outputs --------------------------------------------------------
	// ClaimantReservation is the floor: the smallest sum the claimant should
	// rationally accept today instead of litigating.
	ClaimantReservation int64 `json:"claimant_reservation"`
	// RespondentReservation is the ceiling: the largest sum the respondent
	// should rationally pay today instead of litigating (capped by capacity).
	RespondentReservation int64 `json:"respondent_reservation"`

	ZOPAExists            bool    `json:"zopa_exists"`
	ZOPALow               int64   `json:"zopa_low"`
	ZOPAHigh              int64   `json:"zopa_high"`
	ZOPAWidth             int64   `json:"zopa_width"`
	RecommendedSettlement int64   `json:"recommended_settlement"`
	FairnessWeight        float64 `json:"fairness_weight"` // position of the recommendation inside the ZOPA

	// ---- Explainability -------------------------------------------------
	Rationale       []string `json:"rationale"`       // human-readable derivation, step by step
	Recommendations []string `json:"recommendations"` // structural suggestions (e.g. instalment plan)
	Provider        string   `json:"provider"`        // which LLM produced the qualitative read
	GeneratedAt     time.Time `json:"generated_at"`
}
