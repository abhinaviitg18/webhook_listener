#!/bin/bash

# Colors for premium CLI feel
BLUE='\033[0;34m'
CYAN='\033[0;36m'
GREEN='\033[0;32m'
NC='\033[0m' # No Color

echo -e "${BLUE}====================================================${NC}"
echo -e "${CYAN}    AGENTHOOK : THE WEBHOOK CONTROL LAYER          ${NC}"
echo -e "${BLUE}====================================================${NC}"
echo ""

# 1. Ensure AgentHermes is installed
if ! command -v hermes &> /dev/null
then
    echo -e "AgentHermes (${CYAN}hermes${NC}) not found. Please install it first:"
    echo "npm install -g @nousresearch/hermes-agent"
    exit 1
fi

# 2. Get configuration from user
echo -e "${CYAN}Authenticating with AgentHook...${NC}"
read -p "Enter your AGENTHOOK_TOKEN: " AGENTHOOK_TOKEN
read -p "Enter your AGENTMAIL_API_KEY: " AGENTMAIL_API_KEY

# 3. Add to hermes .env if not already there
HERMES_ENV="$HOME/.hermes/.env"
mkdir -p "$HOME/.hermes"
touch "$HERMES_ENV"

# Use a temporary file to update env to avoid duplicates and ensure values are set
sed -i '' '/AGENTHOOK_TOKEN/d' "$HERMES_ENV"
sed -i '' '/AGENTMAIL_API_KEY/d' "$HERMES_ENV"
echo "AGENTHOOK_TOKEN=$AGENTHOOK_TOKEN" >> "$HERMES_ENV"
echo "AGENTMAIL_API_KEY=$AGENTMAIL_API_KEY" >> "$HERMES_ENV"

# 4. Add the cron job with the new smart scheduling prompt
echo -e "${CYAN}Installing Autonomous Monitor...${NC}"
hermes cron add "*/5 * * * *" "1. Fetch webhooks from https://app.agenthook.store/api/events/by-time?window=5m using AGENTHOOK_TOKEN.
2. IF DATA IS EMPTY: Respond with exactly '[SILENT]'.
3. IF DATA EXISTS:
   a. For each message, use the 'deterministic-processor' skill to categorize and extract data.
   b. If the skill fallbacks to Codex and you encounter a Quota/Token exhaustion error:
      - Send email to abhinaviitg18@gmail.com with exactly: 'Tokens are over there. There is a bug but that will be sent to Codex after this time.'
      - SMART SCHEDULING: Identify the credit replenishment time and use 'hermes cron edit --schedule' to delay until then.
      - Stop processing for this run.
   c. If Codex is successful, ensure the new deterministic script is saved to 'processors/'.
   d. Compile a summary of all processed messages and email to abhinaviitg18@gmail.com using 'agentmail-send'." --name "agenthook-heartbeat"

echo ""
echo -e "${GREEN}✓ Success! Heartbeat configured to run every 5 minutes.${NC}"
echo -e "You can monitor your autonomous agent with: ${CYAN}hermes cron list${NC}"
echo -e "${BLUE}====================================================${NC}"
