# Hackathon Post Notes: Aalsi

*Auto-assembled notes from evaluation harness, architecture insights, and failure analyses for the project submission write-up.*

---

## 1. Problem & Core Thesis
College students miss crucial deadlines not because they don't care, but because student WhatsApp groups have a 99:1 noise-to-signal ratio. Commercial LLMs with cloud backends are a privacy non-starter: students will not (and should not) upload private group chats with personal numbers and faculty chat logs to external cloud endpoints.

**Aalsi proves that open-weight local models (Gemma) combined with deterministic systems code (Go) can deliver a local-first, zero-cost, private deadline catcher.**

---

## 2. Key Architecture Takeaways: "The Model Reads, The Code Decides"

1. **Don't Let the Model Do Calendar Math**:
   - Initial experiments attempting to let LLMs output ISO timestamps (`2026-10-25T17:00:00Z`) from *"next Monday tak"* produced hallucinations whenever the message's historical timestamp differed from the current wall clock.
   - Forcing Gemma to output verbatim phrases (`"kal"`, `"Friday"`, `"28th"`) and delegating calendar resolution to Go in `pkg/resolver` eliminated hallucinated dates entirely.

2. **Deduplication Must Be Multi-Tiered**:
   - Level 1: SHA-256 hash over `timestamp + sender + text` provides 100% idempotency for re-imported chat logs.
   - Level 2: Entity matching across repeated reminders (`"fee payment last date 28th"`) merges reminders into a living timeline instead of flooding the student with duplicate items.
   - Level 3: Guard against similar assignment collisions: assignment numbers (`Assignment 1` vs `Assignment 2`) are isolated with regex to strictly block false-positive merging.

3. **Noise Filtering Saves Massive Compute**:
   - College chats are 70%+ "ok sir", stickers, laughter, and emojis.
   - Pure Go rule-based prefiltering in `pkg/prefilter` drops 70% of chat volume before it ever hits Ollama, cutting inference runtime and power consumption by ~3x on battery-powered student laptops.

---

## 3. Evaluation Observations (Summary from `eval/RESULTS.md` & `eval/failures.md`)

- **Regex Baseline (No Model)**:
  - While fast (0.02s), naive regex search suffers from a staggering **73% false positive noise rate**, creating items out of chat chatter, casual discussions, and questions (*"assignment kab due hai?"*).
  - Date accuracy was 0% on relative and contextual phrasing without surrounding semantics.
- **Gemma Integration**:
  - Open-weight Gemma running locally eliminates casual noise and correctly isolates real faculty announcements.
  - Chunk overlap (~500 tokens) proves vital when an announcement and its immediate student clarification span across chunk limits.

---

## 4. Privacy & Offline Guarantee
- Zero outbound telemetry or external APIs.
- SQLite database stored locally in `~/.aalsi/aalsi.db`.
- Structured logging masks all message content.
- Clean one-command purge (`aalsi purge`) wipes all local tables.
