# Architecture

## Component Diagram

```mermaid
graph TB
    subgraph "Client Layer"
        UI["Single-page UI<br>(web/index.html)<br>Vanilla JS, no framework"]
    end

    subgraph "HTTP Layer  (internal/api)"
        SRV["Server<br>Go 1.22 ServeMux<br>method + path routing"]
        MW["Middleware<br>CORS · Logging · Panic recovery"]
        HDL["Handlers<br>JSON decode/encode<br>error → status mapping"]
    end

    subgraph "Application Layer"
        SVC["Service<br>(internal/api/service.go)<br>Orchestrates state transitions"]
        SEED["Seed<br>3 demo disputes at boot"]
    end

    subgraph "Domain Engines"
        AE["Analysis Engine<br>(internal/analysis)<br>Settlement intelligence +<br>deterministic economic model"]
        NE["Negotiation Engine<br>(internal/negotiation)<br>Double-blind-bid protocol<br>+ BATNA-grounded nudges"]
        DR["Drafter<br>(internal/drafting)<br>Settlement agreement<br>generation"]
    end

    subgraph "Infrastructure"
        STORE["In-Memory Store<br>(internal/store)<br>sync.RWMutex<br>concurrent-safe"]
        LLM["LLM Provider<br>(internal/llm)"]
    end

    subgraph "LLM Providers"
        MOCK["Offline Model<br>(mock.go)<br>Deterministic, no network"]
        ANTH["Anthropic Provider<br>(anthropic.go)<br>Claude via REST API"]
    end

    subgraph "Configuration"
        CFG["Config<br>(internal/config)<br>Env vars + provider selection"]
    end

    UI -->|"same-origin fetch<br>/api/v1/*"| SRV
    SRV --> MW --> HDL --> SVC
    SVC --> AE & NE & DR & STORE
    AE & NE & DR --> LLM
    LLM -->|"key present"| ANTH
    LLM -->|"no key / forced"| MOCK
    CFG -->|"selects provider"| LLM
    SEED --> SVC

    classDef engine fill:#e7d4a6,stroke:#b4842b,color:#14173a
    classDef infra fill:#e9ebf1,stroke:#565a82,color:#14173a
    classDef ui fill:#d1e7dd,stroke:#2f7d5b,color:#14173a
    class AE,NE,DR engine
    class STORE,LLM,MOCK,ANTH,CFG infra
    class UI ui
```

## Sequence Diagram: Full Dispute Lifecycle

```mermaid
sequenceDiagram
    participant U as User / UI
    participant H as HTTP Handlers
    participant S as Service
    participant A as Analysis Engine
    participant LLM as LLM Provider
    participant N as Negotiation Engine
    participant D as Drafter
    participant ST as Store

    Note over U,ST: 1. INTAKE
    U->>H: POST /api/v1/disputes {category, parties, amount, narrative}
    H->>S: CreateDispute(input)
    S->>ST: Create(dispute)
    ST-->>S: dispute (status=intake, id=SAMA-xxx)
    S-->>H: dispute
    H-->>U: 201 Created

    Note over U,ST: 2. ANALYSIS (Settlement Intelligence)
    U->>H: POST /api/v1/disputes/{id}/analyze
    H->>S: Analyze(ctx, id)
    S->>ST: Get(id)
    S->>A: Analyze(ctx, dispute)
    A->>LLM: CompleteJSON[assessment](case facts)
    LLM-->>A: {claim_strength, recovery_rate, capacity_ratio, ...}
    Note over A: Sanitize + clamp model outputs
    Note over A: Run deterministic economic model:<br>pvFactor, floor (BATNA), ceiling (exposure),<br>ZOPA, fairness-weighted recommendation
    Note over A: Build line-by-line rationale<br>Build structural recommendations
    A-->>S: CaseAnalysis
    S->>ST: Update (status=analyzed)
    S-->>H: dispute + analysis
    H-->>U: 200 OK

    Note over U,ST: 3. NEGOTIATION (Double-Blind Bids)
    U->>H: POST /api/v1/disputes/{id}/offers {ask, pay}
    H->>S: SubmitOffers(ctx, id, ask, pay)
    S->>N: Start(dispute, maxRounds) — if first round
    S->>N: SubmitRound(ctx, dispute, ask, pay)

    alt Offers cross (gap ≤ 0)
        N-->>S: Round(outcome=settled, amount=midpoint)
        S->>D: Draft(ctx, dispute, amount, "negotiated")
        D->>LLM: CompleteJSON[draft](facts)
        LLM-->>D: {agreement_text}
        D-->>S: Settlement
        S->>ST: Update (status=settled)
    else Gap remains, rounds left
        N->>LLM: NudgeRequest(claimant's BATNA, their offer)
        LLM-->>N: nudge for claimant (private)
        N->>LLM: NudgeRequest(respondent's exposure, their offer)
        LLM-->>N: nudge for respondent (private)
        N-->>S: Round(outcome=continue, nudges=[claimant, respondent])
    else Final round, gap remains
        N-->>S: Round(outcome=escalated)
        S->>ST: Update (status=escalated)
    end

    S-->>H: {round, dispute}
    H-->>U: 200 OK

    Note over U,ST: 3b. FAST PATH — Accept Recommended
    U->>H: POST /api/v1/disputes/{id}/accept
    H->>S: AcceptRecommended(ctx, id)
    S->>D: Draft(ctx, dispute, recommended, "mediator_proposal")
    D-->>S: Settlement
    S->>ST: Update (status=settled)
    S-->>H: dispute
    H-->>U: 200 OK
```

## Functional Flow: Economic Model

```mermaid
flowchart TD
    START(["Dispute facts"])
    LLM["LLM qualitative assessment<br>claim_strength (p), recovery_rate,<br>time_to_resolution, capacity_ratio"]
    CLAMP["Sanitize & clamp outputs<br>into valid ranges"]

    subgraph "Deterministic Economic Model"
        PV["pvFactor = 1/(1+0.12)^years"]
        EG["expectedGross = claim × p"]
        EC["expectedCollectable = gross × recoveryRate"]
        FLOOR["Claimant floor (BATNA)<br>= max(0, collectable×pvFactor − claimantCost)"]
        CEIL_RAW["respondentExposure<br>= gross×pvFactor + respondentCost"]
        CAP["capacity = claim × capacityRatio"]
        CEIL["Respondent ceiling<br>= min(capacity, exposure)"]
        ZOPA{"ceiling ≥ floor?"}
        YES["ZOPA exists<br>zone = [floor, ceiling]<br>width = ceiling − floor"]
        NO["NO ZOPA<br>floor exceeds ceiling"]
        FAIR["fairnessWeight = clamp(0.35 + 0.30×p)"]
        REC["recommended = floor + width × fairnessWeight<br>rounded to ₹1,000"]
        REC_CAP["recommended = ceiling<br>(capacity-limited fallback)"]
    end

    RAT["Line-by-line rationale<br>with ₹ figures"]
    RECS_Z["Recommendations:<br>zone narrow → settle quickly<br>zone wide → use protocol"]
    RECS_N["Recommendations:<br>instalment plan to open a zone<br>escalate to human neutral"]

    START --> LLM --> CLAMP --> PV & EG
    EG --> EC --> FLOOR
    PV --> FLOOR & CEIL_RAW
    EG --> CEIL_RAW
    CEIL_RAW --> CEIL
    CAP --> CEIL
    FLOOR --> ZOPA
    CEIL --> ZOPA
    ZOPA -->|Yes| YES --> FAIR --> REC --> RAT --> RECS_Z
    ZOPA -->|No| NO --> REC_CAP --> RAT --> RECS_N

    classDef model fill:#e7d4a6,stroke:#b4842b
    classDef result fill:#d1e7dd,stroke:#2f7d5b
    classDef fail fill:#f2d5d2,stroke:#b04a3f
    class PV,EG,EC,FLOOR,CEIL_RAW,CAP,CEIL,FAIR,REC model
    class YES,RECS_Z result
    class NO,REC_CAP,RECS_N fail
```

## Data Flow: Confidential Nudge Protocol

```mermaid
flowchart LR
    C["Claimant<br>submits ask<br>(private)"]
    R["Respondent<br>submits pay<br>(private)"]
    ENG["Engine<br>computes gap"]
    NC["Nudge for Claimant<br>grounded in claimant's<br>own BATNA only"]
    NR["Nudge for Respondent<br>grounded in respondent's<br>own exposure only"]

    C --> ENG
    R --> ENG
    ENG -->|"private channel"| NC -->|"🔒"| C
    ENG -->|"private channel"| NR -->|"🔒"| R

    ENG -.->|"❌ NEVER reveals<br>other party's figure"| NC
    ENG -.->|"❌ NEVER reveals<br>other party's figure"| NR

    classDef priv fill:#e9ebf1,stroke:#565a82
    class NC,NR priv
```

## Technology Choices

| Layer | Choice | Rationale |
|---|---|---|
| Language | Go 1.22 | stdlib-only server, method routing, strong concurrency, single binary |
| External deps | Zero | Entire dependency tree is Go stdlib; only outbound HTTP call is to Anthropic |
| UI | Vanilla JS, single file | No build step, no node_modules, serves from `web/index.html` |
| Storage | In-memory (sync.RWMutex) | Appropriate for a demo / take-home; swap for Postgres behind an interface |
| LLM | Provider interface | `MockProvider` for deterministic offline; `AnthropicProvider` for live |
| Config | Environment variables | 12-factor; zero-config default boots offline with seed data |
