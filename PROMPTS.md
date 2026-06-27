# Prompts & Process

This document covers both the development process (how the product was built with AI assistance) and the actual system prompts the product itself uses to communicate with the language model.

---

## Part 1: Development Process

### Problem Selection

The brief asked to pick a challenging real problem that Presolv360 has tried to solve. I started by researching Presolv360's actual operations:

- Web searches for Presolv360's revenue, team size, advisory council (which includes ex-CJI U.U. Lalit and Justice Srikrishna), and product capabilities.
- Identified their core use case: high-volume BFSI disputes (NBFC/bank loan defaults, cheque-bounce under s.138 NI Act, e-commerce).
- Read their positioning: "not a law firm, does not give legal advice" — advisory/decision-support only.

The problem I landed on: **ODR can't scale because every case needs a scarce human neutral, respondents rarely engage, and no party knows a defensible settlement range.** This is a structural problem, not a feature request.

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
7. Seed data (three cases targeting the three demo branches)
8. Frontend (single-file vanilla JS)
9. Tests, ops files, documentation

### AI-Assisted Development

The entire codebase was built in collaboration with Claude. Key uses:

- **Architecture**: discussing the hybrid model trade-offs, BATNA/ZOPA math, nudge confidentiality architecture
- **Code generation**: producing Go packages file by file with explicit struct definitions, JSON tags, and error handling
- **Testing**: designing test cases for invariant properties (ZOPA consistency, monotonicity, confidentiality)
- **Frontend**: building the ZOPA spectrum SVG visualization and the judicial-ledger aesthetic
- **Research**: web searches for Presolv360's business context, Indian legal concepts (s.138 NI Act, ODR frameworks)
- **Documentation**: this file, the README, EXPLANATORY, ARCHITECTURE, DEMO_SCRIPT

---

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
