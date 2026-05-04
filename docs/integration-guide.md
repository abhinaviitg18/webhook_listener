# Integration Guide: AgentHermes & OpenClaw

This guide explains how to integrate **AgentHook** with **AgentHermes** (for local autonomous operations) and **OpenClaw** (for downstream automation filtering).

---

## 1. AgentHermes Integration

[AgentHermes](https://hermes-agent.nousresearch.com/) is a local AI agent by Nous Research that can autonomously monitor webhooks and perform coding tasks via the Codex CLI.

### Prerequisites
- Python 3.11+
- Node.js (for browser tools)
- [uv](https://github.com/astral-sh/uv) package manager

### Setup Steps

1. **Install AgentHermes:**
   ```bash
   curl -fsSL https://hermes-agent.nousresearch.com/install.sh | bash
   source ~/.zshrc
   ```

2. **Configure Credentials:**
   Edit `~/.hermes/.env` to include your AgentHook token and OpenRouter key:
   ```bash
   OPENROUTER_API_KEY=sk-or-v1-...
   AGENTHOOK_TOKEN=your_agenthook_token
   CODEX_HOME=/path/to/your/project
   # Switch to Groq/Cerebras if OpenRouter credits are low:
   # OPENAI_API_KEY=gsk_...
   # HERMES_INFERENCE_PROVIDER=custom
   # HERMES_INFERENCE_BASE_URL=https://api.groq.com/openai/v1
   ```

3. **Install Codex CLI:**
   AgentHermes uses Codex for autonomous coding.
   ```bash
   npm install -g @openai/codex
   ```

4. **Enable Heartbeat Polling:**
   Schedule Hermes to fetch webhooks every 5 minutes:
   ```bash
   hermes cron add "*/5 * * * *" "Fetch latest webhooks from https://app.agenthook.store/api/events/by-time?window=5m using the AGENTHOOK_TOKEN in the Authorization: Bearer header and analyze them for any needed code changes using Codex. After analysis, send an email to abhinaviitg18@gmail.com via agentmail.to (API: https://api.agentmail.to/v0, using AGENTMAIL_API_KEY) containing: 1) the number of webhooks received, 2) a summary of the events, and 3) what needs to be done for them. If no webhooks were received, respond with [SILENT] and do not send an email." --name "webhook-monitor" --workdir "/path/to/your/project"
   ```

5. **Start the Background Service:**
   ```bash
   hermes gateway install
   hermes gateway status
   ```

---

## 2. OpenClaw Integration

[OpenClaw](https://openclaw.example.com) is an automation platform. AgentHook serves as a high-signal filter for OpenClaw, significantly reducing token costs through two primary patterns:

### Integration Patterns

1. **Deterministic Pull (Recommended):**
   OpenClaw can hit AgentHook's deterministic API endpoints (like `GET /api/events/by-time`) to fetch only pre-classified, high-signal events. This prevents OpenClaw from waking up and consuming tokens when there is only "noise" or "heartbeats" in the stream.

2. **Trigger-Based Push:**
   AgentHook can proactively `push` summarized events to OpenClaw's intake URL as they arrive and pass classification filters.

### Setup Steps (Pull Model)

1. **Obtain API Key:**
   Get your `AGENTHOOK_TOKEN` from the dashboard.

2. **Configure OpenClaw Poll:**
   Set up a recurring job to fetch events:
   ```bash
   curl -H "Authorization: Bearer $AGENTHOOK_TOKEN" \
     "https://app.agenthook.store/api/events/by-time?window=10m"
   ```

3. **Process Events:**
   Iterate through the returned events and trigger downstream workflows.

---

## Cost Optimization

Using AgentHook as a pre-filter for "heartbeat" agents (like Hermes or OpenClaw) saves significant costs:

| Strategy | Token Consumption | Estimated Cost |
| :--- | :--- | :--- |
| **Direct Stream** | High (Every Event) | $$$ |
| **AgentHook Filter** | Low (Filtered Only) | $ |

*Note: Pre-filtering ensures LLMs only process high-intent signals.*
