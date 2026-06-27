# Notes

## Overview

Samādhān — an AI-native Settlement Intelligence & Autonomous Negotiation Engine. It resolves high-volume disputes (loan defaults, cheque bounces, e-commerce claims) by computing a transparent, auditable settlement range and running a confidential double-blind bidding protocol, without requiring a human neutral for routine cases.

## Problem Context

Presolv360's core bottleneck: every dispute needs a scarce neutral, respondents rarely engage, and negotiations stall. This is a genuine scaling problem, not just a feature gap — and the AI-native answer is not "throw a chatbot at it" but a careful hybrid of qualitative AI assessment and deterministic, explainable economics.

## Key Design Decisions

1. **Hybrid architecture.** The LLM produces qualitative inputs (claim strength, recovery rate, capacity). A deterministic economic model produces the settlement range. This is the only defensible design for a legal product — you must be able to answer "why this number?" line by line.

2. **Offline-first.** The system runs end-to-end with a deterministic provider and no network. The real model is a single env var away. This is deliberate for demo, testing, and CI.

3. **Confidential nudges.** Each party's nudge is grounded only in their own alternative. The code enforces this architecturally, not by prompt.

4. **Escalation.** The system knows when to stop: no ZOPA → instalment + human neutral. Max rounds → escalation. No forced deals.

## Production Path

- Persistent storage (Postgres) with per-party auth so each side only sees their own nudges
- Document OCR/extraction pipeline feeding the LLM
- Historical outcome calibration for the economic priors
- Separate real-time channels for each party
- E2E integration tests as Go `httptest` handlers
- Production observability (structured logging + metrics)
