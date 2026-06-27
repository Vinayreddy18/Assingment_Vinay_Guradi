# Explanatory Document

A deep walkthrough of how Samādhān works and why every design decision was made. Written so you can explain the system confidently in an interview — not just what it does, but *why* this way and not another way.

---

## The Problem Samādhān Solves

Presolv360 handles thousands of disputes (primarily BFSI: loan defaults, cheque bounces, e-commerce). The bottleneck: every case needs a scarce human neutral, respondents rarely engage, and negotiations stall because neither party knows a defensible settlement range. An AI that just "suggests a number" is useless because no regulator, court, or case manager will trust a black-box prediction.

Samādhān removes the neutral from the critical path for the bulk of routine cases by:
1. Computing a transparent, auditable settlement range — not a prediction but a *derivation* each side can follow line by line.
2. Running a structured negotiation protocol that keeps both parties honest without revealing confidential positions.
3. Drafting the agreement the moment the matter resolves.

---

## Why Hybrid: LLM Qualitative + Deterministic Economics

This is the most important design choice. Here's the reasoning:

**What the LLM is good at:** Reading messy, unstructured case narratives and documents to produce a qualitative judgment — how strong is the claim, what are the key issues, how long will it take, how likely is recovery. No rules engine can do this well across arbitrary text.

**What the LLM is bad at:** Producing defensible *numbers*. An LLM that says "I recommend ₹1,40,000" gives you no way to audit *why* that number and not ₹1,38,000 or ₹1,45,000. A regulator, a mediator, or a lawyer will ask: "On what basis?" and a neural network's latent space is not an answer.

**The hybrid:** The LLM produces the *inputs* (claim strength, recovery rate, capacity ratio, time estimate). A transparent, deterministic economic model — implemented in plain Go with named constants and a step-by-step rationale — transforms those inputs into the settlement range. Every figure in the rationale can be traced back to a named constant or a model output. The model can be wrong; the derivation is always right given its inputs. This is defensible.

---

## The Economic Model, Worked Through

Let's walk through the cheque-bounce demo case (₹2,50,000 claim) with the offline provider's outputs:

### Inputs from the LLM (or offline model)
- **claim_strength (p)** = 0.94 — very strong; cheque + demand notice + invoice
- **recovery_rate** = 0.60 — collectability if the claimant wins
- **time_to_resolution** = 1.8 years
- **capacity_ratio** = 0.65 — the respondent can pay 65% of the claim

### Named constants (in `analysis/engine.go`)
- **discountRate** = 0.12 (12% annual, reflecting cost of capital tied up in litigation)
- **claimantCostRate** = 0.08 (8% of claim, floor ₹15,000)
- **respondentCostRate** = 0.06 (6% of claim, floor ₹15,000)

### Step 1: Present-value factor
```
pvFactor = 1 / (1 + 0.12)^1.8 = 1 / 1.2239 ≈ 0.817
```
This discounts a future, uncertain recovery into today's rupees.

### Step 2: Claimant's floor (BATNA)
The smallest sum the claimant should rationally accept *today* instead of litigating:
```
expectedGross     = 250000 × 0.94     = 235,000
expectedCollectable = 235000 × 0.60   = 141,000
claimantCost      = max(15000, 250000 × 0.08) = 20,000
claimantBATNA     = 141000 × 0.817 − 20000 = 115,197 − 20,000 = 95,197
claimantReservation = max(0, 95197) ≈ 94,981 (rounding from floats)
```
Why accept below the claim? Because litigation takes 1.8 years, costs ₹20,000, has a 6% chance of losing entirely, and even winning only recovers 60%. The *present value* of that gamble is ₹94,981.

### Step 3: Respondent's ceiling (exposure)
The largest sum the respondent should rationally pay *today* to make the risk disappear:
```
respondentExposure = expectedGross × pvFactor + respondentCost
                   = 235000 × 0.817 + 15000 = 191,995 + 15,000 = 206,995
capacity           = 250000 × 0.65 = 162,500
respondentReservation = min(162500, 206995) = 162,500
```
Capacity is the binding constraint here — the respondent would *rationally* pay up to ₹206,995 to avoid the risk, but can only fund ₹162,500.

### Step 4: ZOPA
```
ceiling (162,500) ≥ floor (94,981) → ZOPA EXISTS
zone = [94,981 — 162,500], width = 67,519
```

### Step 5: Recommendation
```
fairnessWeight = clamp(0.35 + 0.30 × 0.94) = clamp(0.632) = 0.632
recommended = 94981 + 67519 × 0.632 = 94981 + 42,672 = 137,653 → rounded to ₹138,000
```
The fairness weight tilts the recommendation inside the zone: a stronger claim settles nearer the respondent's ceiling.

### When there's NO ZOPA (the distressed-loan case)
When the respondent pleads financial distress, the model (via `mentionsDistress()`) forces `capacityRatio = 0.20`. This pushes the ceiling well below the floor. The engine detects this and:
- Sets `recommended = ceiling` (the most the respondent can bear)
- Fires a recommendation for an **instalment plan** (spreading the payment over 9–18 months raises effective capacity)
- Fires a recommendation to **escalate to a human neutral** if no structural remedy creates an overlap

This is the most legally sophisticated branch, and it shows the system knows when to *stop and defer to a human*.

---

## The Negotiation Protocol

### Why double-blind bids?
If the claimant knows the respondent offered ₹1,10,000, they'll anchor on that number and never come down past ₹1,20,000. If the respondent knows the claimant is asking ₹1,60,000, they'll offer less. Anchoring destroys surplus and stalls deals.

The double-blind protocol: both sides submit a confidential figure. The engine reveals *only* whether they crossed (settle at the midpoint) or whether a gap remains (with private nudges). Neither party ever sees the other's number.

### The confidentiality guarantee
Each party's nudge is grounded *only in their own alternative to settling*:
- The claimant is told: "The present value of pursuing this in litigation is about ₹X" (their BATNA) — and suggested to move toward the fair figure.
- The respondent is told: "Your exposure if this is adjudicated is about ₹Y" (their risk) — and suggested to offer more.

The system *never* reveals the other party's figure. This is tested explicitly: `TestSubmitRound_NudgesAreConfidential` asserts that neither party's nudge message contains the other's FormatINR string.

### Escalation tripwire
After `MaxRounds` (default 3) with a gap remaining, the system escalates to a human neutral. This is deliberate — the system must know its limits and hand off rather than forcing a bad deal.

---

## The Offline Provider

The `MockProvider` is not a test stub. It:
- Reads the *same* structured facts the real model receives
- Uses category-specific base strengths (cheque_bounce = 0.80, loan_default = 0.72, etc.)
- Applies document-type boosts and denial-keyword penalties
- Uses FNV hashing of the narrative for stable per-case jitter
- Detects financial distress keywords to lower capacity
- Produces the same JSON schema the real model returns

This means the entire system — analysis, negotiation, drafting — works end-to-end offline with deterministic, reproducible outputs. You can demo the product, run CI, and verify the economics without touching the network. The real model lights up when `ANTHROPIC_API_KEY` is set.

---

## File-by-File Guide

| File | What it does | Why it's interesting |
|---|---|---|
| `domain/dispute.go` | Aggregate root: `Dispute` with `Category`, `Party`, `Document`, status enum | Zero dependencies; the domain can be reasoned about and tested in isolation |
| `domain/analysis.go` | `CaseAnalysis` struct: qualitative + economic inputs + outputs + rationale | The hybrid: LLM fields and deterministic fields on the same struct |
| `domain/negotiation.go` | `Negotiation`, `Round`, `Nudge`, `Settlement` | Captures the full protocol state including private nudges |
| `domain/errors.go` | Sentinel errors + `FormatINR` (Indian digit grouping) | Indian lakh/crore grouping is load-bearing — it appears in nudges, rationale, and the agreement |
| `llm/llm.go` | `Provider` interface + `CompleteJSON[T]` generic + `extractJSON` | The generic function tries once, retries on bad JSON, and uses a brace-matching parser to extract JSON from prose |
| `llm/prompts.go` | System prompt + request builders for each task | The system prompt explicitly says "you are neutral, not an advocate" and pins the output JSON schema |
| `llm/mock.go` | Deterministic offline provider | Category priors, document-type boosts, distress detection, nudge/draft handlers |
| `llm/anthropic.go` | stdlib `net/http` POST to Anthropic Messages API | No SDK dependency; sends `x-api-key` + `anthropic-version` headers |
| `analysis/engine.go` | The Settlement Intelligence Engine | Named constants, full economic model, `buildRationale` (line-by-line ₹ explanation), `buildRecommendations` (structural remedies) |
| `negotiation/engine.go` | Double-blind-bid protocol | `Start`, `SubmitRound` (settle/continue/escalate), `nudge` with fallback, `SimulateParties` for demo convergence |
| `drafting/drafter.go` | Settlement agreement generation | LLM-powered with a deterministic fallback template |
| `api/service.go` | Application orchestration | The *only* place that mutates dispute state; coordinates the three engines |
| `api/handlers.go` | HTTP handlers | Thin: decode → delegate → encode |
| `api/middleware.go` | CORS, logging, panic recovery, error mapping | Sentinel errors → HTTP status codes in one `switch` |
| `api/server.go` | Go 1.22 method routing + SPA serving | `"POST /api/v1/disputes/{id}/analyze"` — no third-party router |
| `api/seed.go` | Three realistic demo disputes | Designed to exercise ZOPA, accept-recommended, and no-ZOPA branches |
| `config/config.go` | Env-based config + provider selection | Zero-config default: offline + seed + port 8080 |
| `store/memory.go` | In-memory store with `sync.RWMutex` | Swap for Postgres behind an interface |
| `web/index.html` | Single-file vanilla-JS UI | ZOPA spectrum SVG, judicial-ledger aesthetic, zero build step |

---

## AI-Native Design Choices

1. **The LLM is a sensor, not a decision-maker.** It reads unstructured text and produces structured inputs. The decision (the settlement range) comes from a transparent model.

2. **Guardrails are architectural, not prompt-based.** Nudge confidentiality is enforced by the code (each nudge function only receives that party's own alternative). The model *cannot* leak the other party's figure because it never sees it.

3. **The system knows when to stop.** No ZOPA → instalment recommendation + escalation to a human. Max rounds exhausted → escalation. These are explicit branches, not failure modes.

4. **Offline-first is a feature.** The product must be demoable, testable, and verifiable without network access. The offline provider produces the *same shape* of output as the real model, so the economic model and protocol logic are exercised identically in both modes.

5. **"Not legal advice" is a design constraint, not a disclaimer.** The system says "decision-support" throughout. The rationale explains the derivation; it never says "you should accept." The agreement says "this is a record of a consent settlement, not legal advice."

---

## Limitations and Production Path

- **Storage:** In-memory; a production system needs Postgres with dispute-level locking.
- **Authentication:** None; a production system needs per-party auth so each side only sees their own nudges.
- **Document processing:** The offline provider works from summaries; a production system needs an OCR/extraction pipeline feeding the LLM.
- **Priors:** The economic constants (discount rate, cost rates, category priors) are hardcoded; a production system calibrates these from historical outcome data.
- **Concurrency model:** The in-memory store is fine for a single-instance demo. Multi-instance needs a shared store with optimistic locking.
- **Real-time negotiation:** Currently synchronous (both offers submitted in one request). A production system separates the channels so each party submits independently and the engine waits for both.
