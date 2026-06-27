# Demo Script

Spoken walkthrough for a 5–7 minute screen recording (Loom or similar). This covers the three seeded cases, each demonstrating a different branch of the system.

---

## Setup

1. Open a terminal.
2. Run: `go run ./cmd/samadhan` (or `make run`).
3. Open `http://localhost:8080` in a browser.
4. The provider pill in the top bar should show **offline model** — the system is running fully offline with no API key and no network, using a deterministic provider.

---

## Talking Points (Screen Recording)

### Opening (30 seconds)

> "This is Samādhān — a Settlement Intelligence and Autonomous Negotiation Engine built for ODR platforms like Presolv360.
>
> The problem: ODR can't scale because every case needs a scarce human neutral, respondents rarely engage, and negotiations stall because no one knows a defensible settlement range.
>
> Samādhān fixes this by combining a qualitative read from a language model with a transparent, deterministic economic model. The LLM reads the case; the math produces the number — every step auditable, line by line."

### Case 1: Cheque Bounce — Full Negotiate-to-Settlement Flow (2.5 minutes)

**Click the cheque bounce case in the docket.**

> "Here's a ₹2,50,000 dishonoured cheque matter — Meridian Supplies versus Kohli Traders. It has a cheque, a demand notice, and an invoice attached. It's at intake — nothing has been computed yet."

**Click "Run analysis."**

> "The settlement intelligence engine just ran. Let me walk you through what it produced.
>
> The spectrum at the top is the signature view. The green marker is the claimant's *floor* — the smallest amount they should rationally accept today instead of litigating. The red marker is the respondent's *ceiling* — the most they should rationally pay. The brass band between them is the Zone of Possible Agreement. The diamond is the recommended figure.
>
> Below, the analysis tab shows the claim strength — 94%, this is a strong, well-documented claim. Recovery rate, time to resolution, and the recommended figure.
>
> Now scroll down to 'How the range was derived.' Every line has a rupee figure and explains *how* it was computed — probability-weighted award, present-value factor, litigation costs, floor, ceiling, zone, recommendation. A case manager, a mediator, or a regulator can follow this line by line. There's no black box.
>
> The recommendations say the zone is wide enough that the double-blind protocol should be used so neither party anchors the other."

**Switch to the Negotiation tab.**

> "Now let's negotiate. I'll enter confidential offers — the claimant asks ₹1,60,000, the respondent offers ₹1,10,000."

**Enter 160000 / 110000 and click Submit.**

> "The gap is ₹50,000. Both sides have received a private nudge — look, each nudge is marked with a lock icon and says 'private.' The claimant's nudge tells them about *their own* litigation value and suggests moving down. The respondent's nudge tells them about *their own* exposure and suggests moving up. Critically, neither nudge contains the other party's figure. This is enforced architecturally: the nudge function only sees that party's own alternative."

**Enter 140000 / 140000 and click Submit.**

> "The offers just crossed — both said ₹1,40,000. The system settles at the midpoint, which is also ₹1,40,000, and you can see the settled star on the spectrum."

**Switch to the Agreement tab.**

> "The agreement was drafted automatically. It's a proper settlement document — parties, recitals, terms, payment clause, mutual release, confidentiality, and a note that this is a consent settlement, not legal advice. Ready to copy and send."

### Case 2: Loan Default — Accept-Recommended Fast Path (1 minute)

**Click the Arclight Finance loan default (₹5,00,000) in the docket.**

> "This is the fast path — a clean loan default where both sides simply accept the engine's recommended figure."

**Click "Run analysis." Then switch to Negotiation and click "Both accept ₹X."**

> "The system produces a ZOPA, both parties accept the recommended number as a mediator's proposal, and the agreement is drafted. No bidding rounds needed — this is how the bulk of routine cases should settle. One click."

### Case 3: Distressed Loan — No ZOPA, Structural Remedy (1.5 minutes)

**Click the Pinnacle Capital loan (₹12,00,000) in the docket.**

> "This is the hard case. The respondent — Verma Textiles — has said their business has shut down and they cannot pay. Let me analyse it."

**Click "Run analysis."**

> "Look at the spectrum: instead of a brass zone, there's a hatched red gap and it says NO OVERLAP. The claimant's floor is above the respondent's ceiling — no lump-sum settlement is rational for both sides.
>
> Now scroll to the recommendations. The engine says: *capacity is the binding constraint — converting the settlement into an instalment plan raises what the respondent can effectively pay and can open a zone that doesn't exist for a single lump sum.* It also recommends escalating to a human neutral if that doesn't work.
>
> This is where the system shows real judgment: it doesn't force a deal that doesn't exist. It diagnoses *why* there's no deal, suggests a structural remedy, and defers to a human when it runs out of value to add."

### Closing (30 seconds)

> "To recap: Samādhān runs fully offline with zero secrets — the deterministic provider exercises every branch. Set `ANTHROPIC_API_KEY` and it lights up with a real language model for richer qualitative reads, but the economic model and the protocol stay the same.
>
> The design is deliberately hybrid: the AI reads the case, the math produces the number. Every figure is derived, every nudge is confidential, and the system knows when to stop and escalate."

---

## Key Claims to Emphasise

1. **Hybrid = defensible.** The LLM is a sensor, not a decision-maker.
2. **Confidentiality is architectural.** The nudge function never sees the other party's figure.
3. **The system knows when to stop.** No ZOPA → instalment + escalation.
4. **Offline-first is a feature.** Demo, test, CI — all without network or secrets.
5. **"Not legal advice" is a design constraint.** Decision-support, not adjudication.
