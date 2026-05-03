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
   ```

3. **Install Codex CLI:**
   AgentHermes uses Codex for autonomous coding.
   ```bash
   npm install -g @openai/codex
   ```

4. **Enable Heartbeat Polling:**
   Schedule Hermes to fetch webhooks every 5 minutes:
   ```bash
   hermes cron add "*/5 * * * *" "Fetch latest webhooks from https://app.agenthook.store/api/events/by-time?window=5m and analyze them for any needed code changes using Codex." --name "webhook-monitor" --workdir "/path/to/your/project"
   ```

5. **Start the Background Service:**
   ```bash
   hermes gateway install
   hermes gateway status
   ```

---

## 2. OpenClaw Integration

[OpenClaw](https://openclaw.example.com) is an automation platform. AgentHook acts as a high-signal filter for OpenClaw, reducing token costs by discarding "heartbeat" noise and routine status updates.

### Integration Patterns

AgentHook typically integrates with OpenClaw via the **Forward Signal** pattern:
1. Webhook arrives at AgentHook.
2. Skill classifies and summarizes the event.
3. High-signal summary is forwarded to OpenClaw via HTTP.

### Setup Steps

1. **Store OpenClaw API Key:**
   Create an integration secret in AgentHook so your skills can use it:
   ```bash
   # Using AgentHook CLI or UI
   Secret Key: openclaw_api_key
   Purpose: OpenClaw Bearer Token
   Secret Value: sk_oc_...
   ```

2. **Create Forward Target:**
   Define OpenClaw as a destination:
   ```bash
   Target Key: openclaw_primary
   Target Type: http
   URL: https://api.openclaw.example/v1/intake
   Auth: Bearer (referenced via secret_ref: openclaw_api_key)
   ```

3. **Configure Filtering Skill:**
   Add a Skill to your Webhook Type to handle the logic. 
   
   **Example Skill Prompt:**
   > "If the payload is a heartbeat or routine status update, set action to `no_action`. If it's a high-priority lead or critical error, summarize the core details and use `forward_http` to `openclaw_primary`."

### Cost Optimization ROI
By using AgentHook as a pre-filter:
- **Save Tokens:** Never process a "heartbeat" in OpenClaw again.
- **Lower Latency:** OpenClaw only wakes up when there is actual work.
- **Structured Data:** OpenClaw receives clean, summarized JSON instead of raw, messy payloads.
