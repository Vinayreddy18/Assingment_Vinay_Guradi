package llm

import (
	"encoding/json"
	"fmt"
)

// This file is the single home for every prompt Samadhan sends. Keeping them
// together makes the model's behaviour reviewable in one place and keeps the
// engines free of prompt strings. Each builder embeds the structured `facts`
// as a JSON block so the real model and the offline model reason over exactly
// the same inputs, and each pins an explicit output schema.

const neutralRole = `You are Samadhan, a neutral settlement-intelligence assistant used by an ` +
	`Online Dispute Resolution institution in India. You are impartial: you do not advocate ` +
	`for either the claimant or the respondent. You assess facts soberly, you never invent ` +
	`evidence, and you are explicit about uncertainty. You are not a lawyer and your output ` +
	`is decision-support, not legal advice.`

func factsBlock(facts map[string]any) string {
	b, err := json.MarshalIndent(facts, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

// CaseAssessmentRequest asks the model for a qualitative read of the dispute:
// how strong the claim is, how collectable it is, and how long adjudication
// would take. These feed the deterministic economic model — the model never
// picks the settlement number itself.
func CaseAssessmentRequest(facts map[string]any) Request {
	prompt := fmt.Sprintf(`Assess the following dispute for the purpose of computing a fair settlement range.

CASE FACTS (JSON):
%s

Estimate, grounding every number in the facts above:
- claim_strength: probability (0.0-1.0) the claimant prevails on the merits if adjudicated.
- confidence: your confidence (0.0-1.0) in this read given the documents provided.
- time_to_resolution_years: realistic years to resolve via court/arbitration in India for this category.
- recovery_rate: fraction (0.0-1.0) of an award actually collectable from the respondent.
- respondent_capacity_ratio: respondent's likely ability to pay, as a fraction (0.0-1.0) of the claim amount.
- key_issues: 2-4 short strings naming the pivotal contested points.
- summary: 2-3 neutral sentences a case manager can read.

Respond with ONLY this JSON object:
{"claim_strength":0.0,"confidence":0.0,"time_to_resolution_years":0.0,"recovery_rate":0.0,"respondent_capacity_ratio":0.0,"key_issues":["..."],"summary":"..."}`,
		factsBlock(facts))

	return Request{
		Task:        TaskCaseAssessment,
		System:      neutralRole,
		Prompt:      prompt,
		Facts:       facts,
		MaxTokens:   700,
		Temperature: 0.2,
	}
}

// NudgeRequest asks for a short, neutral message that encourages one party to
// move. The message must be grounded only in that party's own alternative to
// settling — never in the other side's confidential figure.
func NudgeRequest(facts map[string]any) Request {
	prompt := fmt.Sprintf(`Write a brief, neutral nudge to one party in a confidential settlement negotiation.

CONTEXT (JSON):
%s

Rules:
- Address the party named in "party".
- Ground the message ONLY in their own position: their alternative to settling (their_alternative),
  the modelled fair range, and the cost/time/risk of not settling.
- NEVER reference or hint at the other party's confidential figure.
- Be respectful and non-coercive. 2-3 sentences.
- suggested_offer must be an integer in rupees that moves them toward the fair range.

Respond with ONLY this JSON object:
{"message":"...","suggested_offer":0}`,
		factsBlock(facts))

	return Request{
		Task:        TaskNudge,
		System:      neutralRole,
		Prompt:      prompt,
		Facts:       facts,
		MaxTokens:   400,
		Temperature: 0.5,
	}
}

// DraftRequest asks for a plain-language settlement agreement built around the
// agreed figure. The legal scaffolding (recitals, clauses) is fixed by the
// template the model is asked to follow so output stays consistent.
func DraftRequest(facts map[string]any) Request {
	prompt := fmt.Sprintf(`Draft a clear settlement agreement for the following resolved dispute.

SETTLEMENT FACTS (JSON):
%s

Produce a complete agreement with: a title, a date line, identification of the parties,
recitals (background of the dispute), the settlement terms (the agreed amount and that it
fully and finally settles the dispute), a mutual release, a confidentiality clause, and
signature blocks. Use plain English. Include a short note that this is a record of a
consent settlement reached through Online Dispute Resolution and is not legal advice.

Respond with ONLY this JSON object (agreement_text may contain newlines):
{"agreement_text":"..."}`,
		factsBlock(facts))

	return Request{
		Task:        TaskDraft,
		System:      neutralRole,
		Prompt:      prompt,
		Facts:       facts,
		MaxTokens:   1200,
		Temperature: 0.3,
	}
}
