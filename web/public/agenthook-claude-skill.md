# AgentHook Features & Integration Guide

AgentHook is a webhook processing platform that sits in front of your noisy systems. It accepts events from apps, email, or internal tools, decides what is noise versus signal, stores a clear record in Storyboard, and forwards only meaningful events into your downstream workflows.

This document serves as a "Claude Skill" — a context prompt designed to help LLMs understand AgentHook's architecture and capabilities.

## Core Concepts

### 1. Ingress Types
AgentHook supports two primary modes of receiving events:
- **HTTP Webhook**: A POST request to `/{public_alias}.{secret}` containing a JSON payload.
- **Email Ingress**: An email sent to `{public_alias}.{secret}@app.agenthook.store`. The email is received via AWS SES, processed by a Lambda, and fed into AgentHook as an event.

### 2. Primary Objects
- **Listener**: The ingress endpoint, defined by a provider (e.g., github, stripe), a listener ID, deployment mode, and a type key.
- **Secret**: Activates both the webhook URL and inbox address for a listener.
- **Skill**: A rule or prompt that classifies, routes, tags, summarizes, or nominates actions for incoming events.
- **Integration**: A named target (like OpenClaw or any custom forward URL) where AgentHook can send filtered events.
- **Event**: A stored Storyboard item containing the raw payload, processed text, action selected, and tags.

## LLM Skills (Routing & Filtering)
AgentHook uses "Skills" to process incoming payloads. A skill contains:
- **Match Criteria**: Determines if a skill should execute on a payload based on the payload's contents.
- **LLM Prompt**: Instructs the LLM on how to classify the event, extract data, or generate a summary.
- **Forced Action**: An optional deterministic action assigned if the skill matches (e.g., `store_mysql`, `slack_notify`, `forward_http`).
- **Memory Write Mode**: Dictates whether the event should be inserted into the database or if it should update an existing record.

## Integrations & Forwarding
AgentHook acts as an intelligent router. It can:
- **Drop Noise**: Filter out heartbeats, low-value health checks, and routine status pings using skills with the `no_action` forced action.
- **Forward Signal**: Send meaningful, summarized JSON payloads to Integrations via HTTP (`forward_http`), Slack (`slack_notify`), or custom targets like OpenClaw (`crm_upsert`).
- **Integration Secrets**: AgentHook manages secrets (e.g., API keys, Bearer tokens) to securely authenticate with downstream integrations without hardcoding them in URLs.

## Deployment Modes
- **Basic / Multitenant**: The standard cloud-hosted version running on `app.agenthook.store`.
- **Enterprise / Single-tenant**: For teams wanting full control, AgentHook can be deployed on custom AWS infrastructure with custom domains.

## Reclassification
A key feature of AgentHook is the ability to retroactively apply new rules. If a webhook payload was classified poorly, users can update their Skills, go to the Storyboard, select the event, and hit "Reclassify". AgentHook will rerun the LLM analysis and apply the new logic.

## Usage Guidelines for AI Agents
When integrating with or managing AgentHook, keep these operations in mind:
1. **Create a Listener**: Set the provider and default action.
2. **Generate a Secret**: Use this to mint the ingress URL or email address.
3. **Write Skills**: Define specific match criteria and LLM prompts to accurately categorize payloads.
4. **Configure Integrations**: Set up forward URLs with the correct authentication headers.
5. **Review Storyboard**: Check the logs to ensure the LLM correctly summarizes and acts upon raw events. Use the reclassification feature to fine-tune.
