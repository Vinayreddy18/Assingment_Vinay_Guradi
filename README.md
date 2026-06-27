# Samādhān

**Settlement Intelligence & Autonomous Negotiation Engine**

An AI-native system that resolves high-volume disputes without a human neutral by combining a transparent economic model with qualitative assessment from a language model. Built for the kind of work [Presolv360](https://presolv360.com) does — BFSI loan defaults, cheque-bounce matters, e-commerce claims — where the bottleneck is not knowing a defensible settlement range, and where respondents rarely engage because no one tells them their exposure in terms they can act on.

Samādhān computes each party's realistic alternative to a negotiated outcome (BATNA), derives the Zone of Possible Agreement, runs a confidential double-blind bidding protocol with private BATNA-grounded nudges, and drafts the settlement agreement the moment offers cross — all auditable, all explainable, line by line.

---

## Quickstart

```bash
# Clone and run (Go 1.22+)
go run ./cmd/samadhan

# Open your browser
open http://localhost:8080
```

That's it. **No API key, no database, no external dependencies.** The service boots with a deterministic offline model and three seeded demo cases.

To use a live model instead:

```bash
# Anthropic
export ANTHROPIC_API_KEY=sk-ant-...
go run ./cmd/samadhan

# Or OpenAI
export OPENAI_API_KEY=sk-...
export SAMADHAN_PROVIDER=openai
go run ./cmd/samadhan
```

### With Docker

```bash
docker build -t samadhan .
docker run --rm -p 8080:8080 samadhan

# With live Anthropic:
docker run --rm -p 8080:8080 -e ANTHROPIC_API_KEY=sk-ant-... samadhan

# With live OpenAI:
docker run --rm -p 8080:8080 -e OPENAI_API_KEY=sk-... -e SAMADHAN_PROVIDER=openai samadhan
```

### With Make

```bash
make run           # run with whatever provider is configured
make run-offline   # force the deterministic offline model
make test          # run all unit tests
make check         # format + vet + test
```

---

## Configuration

All settings are environment variables with sensible defaults:

| Variable | Default | Purpose |
|---|---|---|
| `ANTHROPIC_API_KEY` | *(empty)* | Set to use the live Anthropic model |
| `OPENAI_API_KEY` | *(empty)* | Set to use the live OpenAI model |
| `SAMADHAN_MODEL` | `claude-sonnet-4-6` | Anthropic model ID |
| `SAMADHAN_OPENAI_MODEL` | `gpt-4.1-mini` | OpenAI model ID |
| `SAMADHAN_PROVIDER` | *(auto)* | Force `mock`, `anthropic`, or `openai` |
| `SAMADHAN_ADDR` | `:8080` | HTTP listen address |
| `SAMADHAN_WEB_DIR` | `web` | Static UI directory |
| `SAMADHAN_MAX_ROUNDS` | `3` | Max bidding rounds before escalation |
| `SAMADHAN_SEED` | `true` | Seed demo disputes on boot |

Provider auto-selection: if `SAMADHAN_PROVIDER` is empty, Anthropic is used when
`ANTHROPIC_API_KEY` is set; otherwise OpenAI is used when `OPENAI_API_KEY` is
set; otherwise Samādhān falls back to the deterministic offline model.

See `.env.example` for a copyable template.

---

## API Reference

All endpoints return JSON. Amounts are whole rupees (int64).

| Method | Path | Description |
|---|---|---|
| `GET` | `/healthz` | Liveness probe |
| `POST` | `/api/v1/disputes` | Create a dispute |
| `GET` | `/api/v1/disputes` | List all disputes |
| `GET` | `/api/v1/disputes/{id}` | Get a single dispute |
| `POST` | `/api/v1/disputes/{id}/analyze` | Run settlement intelligence |
| `POST` | `/api/v1/disputes/{id}/offers` | Submit one round of confidential bids |
| `POST` | `/api/v1/disputes/{id}/simulate` | Auto-simulate a negotiation (demo) |
| `POST` | `/api/v1/disputes/{id}/accept` | Both sides accept the recommended figure |

### Create dispute (POST /api/v1/disputes)

```json
{
  "category": "cheque_bounce",
  "title": "Dishonoured cheque — A v. B",
  "claimant": { "name": "A Pvt Ltd" },
  "respondent": { "name": "B Traders" },
  "claim_amount": 250000,
  "narrative": "A cheque was dishonoured; demand notice served.",
  "respondent_response": "",
  "documents": [
    { "name": "Cheque", "type": "cheque", "summary": "Returned unpaid" }
  ]
}
```

Categories: `loan_default`, `cheque_bounce`, `ecommerce`, `rent_tenancy`, `service_deficiency`, `generic`.

### Submit offers (POST /api/v1/disputes/{id}/offers)

```json
{ "claimant_ask": 160000, "respondent_pay": 110000 }
```

Returns `{ "round": { ... }, "dispute": { ... } }`. The round contains the outcome (`continue`, `settled`, `escalated`) and private nudges. Neither party's figure is revealed to the other — this is the core confidentiality guarantee.

---

## Running Tests

```bash
go test ./...        # unit tests (hermetic, offline, no network)
go test -race ./...  # with the race detector
go test -cover ./... # with coverage
```

Tests cover the economic model invariants (ZOPA consistency, monotonicity, determinism, input validation), the negotiation protocol (settle-on-cross, escalation, nudge confidentiality, simulation convergence), the JSON extractor (fences, nested braces, escaped quotes), and the Indian currency formatter.

---

## Project Structure

```
samadhan/
├── cmd/samadhan/main.go          # entrypoint
├── internal/
│   ├── domain/                   # entities, errors, FormatINR
│   ├── llm/                      # provider interface, Anthropic/OpenAI, offline mock, prompts
│   ├── analysis/                 # settlement intelligence engine (economic model)
│   ├── negotiation/              # double-blind-bid protocol engine
│   ├── drafting/                 # settlement agreement drafter
│   ├── config/                   # env-based configuration
│   ├── store/                    # in-memory concurrent store
│   └── api/                      # HTTP server, handlers, middleware, seed data
├── web/index.html                # single-file UI (vanilla JS, no framework)
├── Makefile / Dockerfile / docker-compose.yml
├── README.md / ARCHITECTURE.md
├── DEMO_SCRIPT.md / NOTES.md / PROMPTS.md
└── go.mod
```

See [ARCHITECTURE.md](ARCHITECTURE.md) for diagrams and [NOTES.md](NOTES.md) for design notes.
