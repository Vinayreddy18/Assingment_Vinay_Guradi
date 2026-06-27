# Prompts

This document covers the system prompts Samādhān uses to communicate with the language model, along with the product design context behind those prompts.

---

## Part 1: Product Context

### Problem Context

Samādhān is designed around Presolv360's high-volume Online Dispute Resolution context:

- High-volume BFSI disputes such as NBFC/bank loan defaults, cheque-bounce matters under s.138 NI Act, and e-commerce claims.
- A platform posture of advisory/decision-support rather than legal representation.
- A need for repeatable settlement intelligence that can scale without depending on a scarce human neutral for every routine case.

The core problem: **ODR can't scale because every case needs a scarce human neutral, respondents rarely engage, and no party knows a defensible settlement range.** This is a structural problem, not a feature request.

### Architecture Decision

Key insight: an AI that just "suggests a number" is useless for a legal product. The suggestion must be *derivable* — a regulator, mediator, or court must be able to trace every figure back to a named input. This led to the hybrid design:

- **LLM** → qualitative inputs (claim strength, recovery rate, capacity, time)
- **Deterministic economic model** → settlement range (BATNA, exposure, ZOPA, recommendation)

The economic model was designed on paper first (BATNA/WATNA framework from negotiation theory) and then implemented with named constants so every step appears in the rationale.

### Build Sequence

1. Domain entities (dispute, analysis, negotiation, settlement)
2. LLM provider interface and offline mock
3. Analysis engine (economic model)
4. Negotiation engine (double-blind protocol)
5. Drafting engine
6. HTTP handlers and server
7. Seed data (three cases targeting the main product branches)
8. Frontend (single-file vanilla JS)
9. Tests, ops files, documentation

## Part 2: System Prompts Used by the Product

These are the actual prompts the product sends to the language model at runtime. They live in `internal/llm/prompts.go`.

### Neutral Role (System Prompt — All Tasks)

```
You are a neutral settlement intelligence engine for an Online Dispute Resolution
platform. Your role is to assess the merits and economics of civil and commercial
disputes so that a transparent, defensible settlement range can be computed. You
are not an advocate for either party. You are not a judge. You provide structured
inputs to a deterministic economic model.

Important:
- Be calibrated. A 0.5 means genuinely uncertain, not hedging. 0.9 means near-certain.
- Ground every assessment in the facts and documents provided.
- If facts are insufficient, lower your confidence rather than guessing.
- Never recommend that a party accept or reject. You produce inputs; the model produces the range.
```

### Case Assessment Request

The assessment prompt sends structured facts (category, claim amount, narrative, respondent response, document types) and asks for:

```json
{
  "claim_strength": 0.0-1.0,
  "confidence": 0.0-1.0,
  "time_to_resolution_years": float,
  "recovery_rate": 0.0-1.0,
  "respondent_capacity_ratio": 0.0-1.5,
  "key_issues": ["string"],
  "summary": "string"
}
```

The prompt explicitly defines each field's semantics and valid range, and tells the model to respond with JSON only.

### Nudge Request

The nudge prompt receives:
- Which party this nudge is for
- **That party's own alternative** (their BATNA or exposure)
- Their current offer
- The recommended settlement

It does **not** receive the other party's offer. This is the architectural enforcement of confidentiality.

The prompt asks for:
```json
{
  "message": "A neutral, empathetic message grounded in THIS party's own alternative...",
  "suggested_offer": integer
}
```

The prompt explicitly says: "Never reference or reveal the other party's offer. Ground the message only in this party's own alternative to settling."

### Draft Request

The drafting prompt receives the case facts, the settlement amount, and the method (negotiated or mediator's proposal), and produces:

```json
{
  "agreement_text": "A formal settlement agreement document..."
}
```

The prompt specifies the structure: parties, recitals, terms (settlement sum, manner of payment, full and final settlement, mutual release, confidentiality, enforceability), signature blocks, and the "not legal advice" disclaimer.

---

## Part 3: Offline Provider Design

The `MockProvider` (`internal/llm/mock.go`) is a deterministic, network-free implementation that produces schema-correct outputs for all three tasks. It is not a test stub — it reads the same structured facts the real model receives and applies transparent heuristics:

**Assessment:**
- Base claim strengths by category (cheque_bounce=0.80, loan_default=0.72, etc.)
- Document-type boosts (cheque +0.06, demand_notice +0.04, etc.)
- Denial-keyword penalties (forged, fraud, never received → -0.05)
- Financial distress detection (keywords like "cannot pay", "shut down", "financial hardship") → forces capacity_ratio to 0.20
- Per-case jitter via FNV hash of the narrative (±0.03)

**Nudges:**
- Moves the suggested offer 40% of the way from the current offer toward the recommended figure
- Produces empathetic, BATNA-grounded messages mentioning the party's exposure/recovery

**Drafting:**
- Fills a complete settlement agreement template with the case details

---

## Part 4: Prompt / Transcript Summary

The development transcript can be summarized into the following prompt groups:

### Problem Framing

```text
Design an AI-native product for Presolv360 that addresses a real Online Dispute Resolution bottleneck. Focus on high-volume disputes where a human neutral is scarce and parties need a defensible settlement range.
```

Outcome: the product direction became a settlement-intelligence and autonomous negotiation engine for routine BFSI, cheque-bounce, and e-commerce disputes.

### Architecture

```text
Propose a defensible architecture for legal/ODR settlement recommendations. The system should not rely on an opaque model-generated number; every settlement figure should be auditable.
```

Outcome: the hybrid architecture was selected:

- LLM for qualitative reading of facts and documents.
- Deterministic economic model for BATNA, exposure, ZOPA, and recommendation.
- Line-by-line rationale for explainability.

### Domain Model

```text
Create Go domain entities for disputes, parties, documents, case analysis, negotiation rounds, nudges, and settlements. Keep the domain independent from HTTP, storage, and model providers.
```

Outcome: the `internal/domain` package defines the aggregate root and lifecycle states: intake, analyzed, negotiating, settled, and escalated.

### LLM Provider Abstraction

```text
Build a provider interface so the system can run offline with deterministic responses and also support live model providers when API keys are available.
```

Outcome: the `llm.Provider` interface supports the offline mock provider and live Anthropic/OpenAI providers.

### Settlement Intelligence Engine

```text
Implement the settlement analysis engine. The LLM should produce qualitative inputs such as claim strength, recovery rate, time to resolution, and capacity. The deterministic model should calculate claimant floor, respondent ceiling, ZOPA, and recommended settlement.
```

Outcome: `internal/analysis/engine.go` implements the economic model with named constants and a generated rationale.

### Confidential Negotiation

```text
Implement a double-blind bidding protocol. Each party submits a confidential figure. If offers cross, settle at the midpoint. If not, send each side a private nudge without revealing the other party's number.
```

Outcome: `internal/negotiation/engine.go` handles settlement, continuation, private nudges, and escalation after max rounds.

### Agreement Drafting

```text
Draft a settlement agreement automatically once a dispute resolves. Include parties, recitals, payment terms, full and final settlement, mutual release, confidentiality, signature blocks, and a note that it is not legal advice.
```

Outcome: `internal/drafting/drafter.go` generates the final settlement agreement, with deterministic fallback text if the model call fails.

### Frontend

```text
Create a no-build single-page UI that lets a user select or create a case, run analysis, view the settlement spectrum, negotiate, accept the recommended figure, and view the final agreement.
```

Outcome: `web/index.html` implements the full user flow in vanilla HTML/CSS/JavaScript.

### Testing

```text
Add focused tests for economic model invariants, deterministic offline behavior, negotiation settlement/escalation, nudge confidentiality, JSON extraction, and INR formatting.
```

Outcome: unit tests cover the highest-risk logic while staying offline and repeatable.
