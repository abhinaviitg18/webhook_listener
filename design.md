# 🪝 AgentHook Design: The Perfect Webhook Experience

## 1. Design Philosophy: "Webhook Zen"
A perfect webhook listener should be **Zero-Config**, **Human-Readable**, and **Action-Oriented**.

1.  **Zero-Config**: You shouldn't need to define a schema before sending data. Send it; let the system learn.
2.  **Human-Readable**: Webhooks are for machines, but logs are for humans. The system should translate JSON blobs into human stories.
3.  **Action-Oriented**: A webhook without an action is just noise. Every event should lead to a decision (Store, Forward, Notify, or Recall).
4.  **Transparent BYOK**: You own your brain. Plugin your own LLM keys (Groq, OpenAI, Anthropic) seamlessly.

---

## 2. The Ingress: "Magic URLs"
Inbound webhooks use a simple, predictable structure:
`https://agenthook.store/url/{account_slug}/{webhook_type}/{secret}`

-   **Auto-Provisioning**: Sending a request to a new `webhook_type` automatically registers it in "Learning Mode".
-   **Secret Rotation**: Support for multiple active secrets per type for zero-downtime rotation.

---

## 3. The Core Experience: "The Learning Loop"
The system utilizes a 3-stage lifecycle for every webhook type:

### Stage 1: Validation (Learning)
-   **Goal**: Gather samples and infer schema.
-   **UX**: User receives a notification: *"I see a new 'stripe-invoice' event. I've inferred the schema and will start shadow-processing."*

### Stage 2: Shadow (Testing)
-   **Goal**: Run LLM logic in parallel with deterministic rules to measure confidence.
-   **UX**: Dashboard shows: *"I would have forwarded this to Telegram, but I'm still in Shadow mode. Click 'Approve' to go live."*

### Stage 3: Active (Production)
-   **Goal**: Full automation.
-   **UX**: Real-time human-readable feed. *"Processed 50 invoices today. 2 required manual review due to low confidence."*

---

## 4. Bring Your Own Key (BYOK)
We empower the user by decoupling the platform from the LLM provider.

-   **Provider Agility**: Swap between Groq (for speed), Anthropic (for reasoning), or OpenAI (for general tasks) with one click.
-   **Cost Transparency**: Use your own API credits. No platform markup on LLM tokens.
-   **Security**: Keys are stored in AWS Secrets Manager, never logged, and used only for your account.

---

## 5. AI Skills: "Plain English Programming"
Instead of writing complex code, users define "Skills":
-   **Skill Example**: *"If the payload contains a 'refund' greater than $100, alert the management team on Slack and summarize the reason."*
-   **Implementation**: LLM parses the intent, matches against the incoming payload, and executes the `forward_http` or `notify` action.

---

## 6. Visibility: "The Storyboard"
The Dashboard replaces traditional "Logs" with a "Storyboard":

| Time | Event Type | The Story | Action Taken |
| :--- | :--- | :--- | :--- |
| 10:00 | `shop-order` | "Alice bought a 'Large Coffee' for $5.00." | `store_mysql` |
| 10:05 | `github-push` | "Bob pushed 3 commits to `main` fixing Bug #4." | `forward_telegram` |
| 10:10 | `unknown` | "Received data from 1.2.3.4. Unknown format." | `manual_review` |

---

## 7. Technical Architecture (High Level)
1.  **Ingest (Go/Gin)**: Validates URL and Auth. Pushes raw payload to SQS.
2.  **Processor (Go Worker)**: 
    -   Fetches Account-specific BYOK config.
    -   Calls LLM (groq/openai) for "Intent Extraction" & "Summarization".
    -   Applies Semantic Memory (Pinecone) for context.
3.  **Forwarder**: Executes side effects (HTTP POST, Telegram, Email).
4.  **Persistence**: RDS (Structured Data) + Pinecone (Semantic Memory).
