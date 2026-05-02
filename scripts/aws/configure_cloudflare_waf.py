#!/usr/bin/env python3
import json
import os
import sys
import urllib.request
import urllib.error
from pathlib import Path

def load_env():
    env_path = Path.cwd() / ".env"
    if not env_path.exists():
        print("Error: .env file not found.")
        sys.exit(1)
        
    for line in env_path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        os.environ[key.strip()] = value.strip()

def cf_request(method, endpoint, token, payload=None):
    url = f"https://api.cloudflare.com/client/v4{endpoint}"
    headers = {
        "Authorization": f"Bearer {token}",
        "Content-Type": "application/json"
    }
    data = json.dumps(payload).encode("utf-8") if payload else None
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    
    try:
        with urllib.request.urlopen(req) as response:
            return json.loads(response.read().decode())
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        print(f"API Error ({e.code}): {body}")
        sys.exit(1)

def main():
    load_env()
    token = os.environ.get("CLOUDFLARE_API_TOKEN_ZONE_SETTING")
    if not token:
        print("Error: CLOUDFLARE_API_TOKEN_ZONE_SETTING not found in .env")
        sys.exit(1)

    print("Fetching Zone ID for agenthook.store...")
    zones_resp = cf_request("GET", "/zones?name=agenthook.store", token)
    if not zones_resp.get("result"):
        print("Error: Could not find zone agenthook.store. Check token permissions.")
        sys.exit(1)
        
    zone_id = zones_resp["result"][0]["id"]
    print(f"Found Zone ID: {zone_id}")

    # Fetch custom rulesets (phase: http_request_firewall_custom)
    print("Fetching Custom WAF ruleset...")
    rulesets_resp = cf_request("GET", f"/zones/{zone_id}/rulesets", token)
    
    ruleset_id = None
    for rs in rulesets_resp.get("result", []):
        if rs.get("phase") == "http_request_firewall_custom":
            ruleset_id = rs.get("id")
            break
            
    if not ruleset_id:
        print("No existing custom firewall ruleset found. Creating one...")
        create_payload = {
            "name": "default",
            "kind": "zone",
            "phase": "http_request_firewall_custom",
            "rules": [{
                "action": "skip",
                "action_parameters": {
                    "phases": ["http_request_sbfm", "http_request_firewall_managed"]
                },
                "expression": '(http.user_agent eq "AgentHook-Forwarder/1.0")',
                "description": "Allow Internal Webhook Forwarder bypass Bot Management"
            }]
        }
        cf_request("POST", f"/zones/{zone_id}/rulesets", token, create_payload)
        print("Successfully created custom ruleset and added the bypass rule!")
    else:
        print(f"Found existing ruleset ID: {ruleset_id}. Adding bypass rule...")
        rule_payload = {
            "action": "skip",
            "action_parameters": {
                "phases": ["http_request_sbfm", "http_request_firewall_managed"]
            },
            "expression": '(http.user_agent eq "AgentHook-Forwarder/1.0")',
            "description": "Allow Internal Webhook Forwarder bypass Bot Management"
        }
        cf_request("POST", f"/zones/{zone_id}/rulesets/{ruleset_id}/rules", token, rule_payload)
        print("Successfully added the bypass rule to existing ruleset!")

if __name__ == "__main__":
    main()
