#!/bin/bash

# AgentHook Heartbeat Setup Script
# This script configures AgentHermes to autonomously monitor your webhooks.

# 1. Ensure AgentHermes is installed
if ! command -v hermes &> /dev/null
then
    echo "AgentHermes (hermes) not found. Please install it first:"
    echo "npm install -g @nousresearch/hermes-agent"
    exit 1
fi

# 2. Get configuration from user
echo "Setting up AgentHook Heartbeat..."
read -p "Enter your AGENTHOOK_TOKEN: " AGENTHOOK_TOKEN
read -p "Enter your AGENTMAIL_API_KEY: " AGENTMAIL_API_KEY

# 3. Add to hermes .env if not already there
HERMES_ENV="$HOME/.hermes/.env"
mkdir -p "$HOME/.hermes"
touch "$HERMES_ENV"

grep -q "AGENTHOOK_TOKEN" "$HERMES_ENV" || echo "AGENTHOOK_TOKEN=$AGENTHOOK_TOKEN" >> "$HERMES_ENV"
grep -q "AGENTMAIL_API_KEY" "$HERMES_ENV" || echo "AGENTMAIL_API_KEY=$AGENTMAIL_API_KEY" >> "$HERMES_ENV"

# 4. Add the cron job
# Note: window=5m for 5-minute intervals
hermes cron add "*/5 * * * *" "1. Fetch webhooks from https://app.agenthook.store/api/events/by-time?window=5m using AGENTHOOK_TOKEN.
2. IF DATA IS EMPTY: Respond with exactly '[SILENT]'.
3. IF DATA EXISTS: Use 'deterministic-processor' skill and Codex fallback to 'processors/' directory.
4. Send summary email via 'agentmail-send'." --name "agenthook-heartbeat"

echo "Success! Heartbeat configured to run every 5 minutes."
echo "You can view your jobs with: hermes cron list"
