#!/usr/bin/env python3
import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path


def load_env_files():
    for name in ("local.env", ".env"):
        path = Path.cwd() / name
        if not path.exists():
            continue
        for line in path.read_text().splitlines():
            line = line.strip()
            if not line or line.startswith("#") or "=" not in line:
                continue
            key, value = line.split("=", 1)
            key = key.strip()
            value = value.strip()
            if key and key not in os.environ:
                os.environ[key] = value


def json_request(method, url, payload=None, token=None, timeout=20):
    headers = {
        "Accept": "application/json",
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
    }
    data = None
    if payload is not None:
        data = json.dumps(payload).encode("utf-8")
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = resp.read().decode("utf-8")
            return resp.status, json.loads(body) if body else {}
    except urllib.error.HTTPError as err:
        body = err.read().decode("utf-8")
        try:
            parsed = json.loads(body) if body else {}
        except json.JSONDecodeError:
            parsed = {"error": body}
        raise RuntimeError(f"{method} {url} -> {err.code}: {parsed}") from err


def register_account(base_url, email):
    _, payload = json_request("POST", f"{base_url}/api/register/email", {"email": email})
    return payload["token"], payload["account"]


def create_listener(base_url, token, provider, listener_id, secret_value, plain_text_action="", use_llm_fallback=False):
    _, payload = json_request("POST", f"{base_url}/v1/listeners", {
        "provider": provider,
        "listener_id": listener_id,
        "deployment_mode": "multitenant",
        "plain_text_action": plain_text_action,
        "use_llm_fallback": use_llm_fallback,
        "secret_value": secret_value,
    }, token=token)
    return payload


def create_forward_target(base_url, token, target_key, sink_webhook_url, source_listener):
    _, payload = json_request("POST", f"{base_url}/api/forward-targets", {
        "target_key": target_key,
        "target_type": "http",
        "purpose": f"relay to {source_listener}",
        "enabled": True,
        "allowed_actions": ["forward_http"],
        "config": {
            "url": sink_webhook_url,
            "headers": {
                "x-agenthook-source-listener": source_listener,
            },
        },
    }, token=token)
    return payload


def create_skill(base_url, token, type_key, skill_key, match_contains, forced_action, skill_prompt, priority=1):
    _, payload = json_request("POST", f"{base_url}/api/policy/skills", {
        "type_key": type_key,
        "skill_key": skill_key,
        "skill_prompt": skill_prompt,
        "match_contains": match_contains,
        "forced_action": forced_action,
        "priority": priority,
        "enabled": True,
    }, token=token)
    return payload


def upsert_byok(base_url, token, provider, api_key, base_url_override="", model=""):
    _, payload = json_request("POST", f"{base_url}/v1/byok/providers", {
        "provider": provider,
        "api_key": api_key,
        "base_url": base_url_override,
        "model": model,
        "is_default": True,
    }, token=token)
    return payload


def post_webhook(webhook_url, payload):
    _, out = json_request("POST", webhook_url, payload)
    return out


def list_events(base_url, token, listener_id, provider):
    _, payload = json_request("GET", f"{base_url}/v1/listeners/{listener_id}/events?provider={urllib.parse.quote(provider)}", token=token)
    return payload


def find_event(events, marker):
    for item in events:
        haystacks = [
            item.get("raw_payload_json", ""),
            item.get("payload_json", ""),
            item.get("processed_text", ""),
        ]
        if any(marker in str(value) for value in haystacks):
            return item
    return None


def poll_for_event(base_url, token, listener_id, provider, marker, timeout_sec=15):
    deadline = time.time() + timeout_sec
    while time.time() < deadline:
        events = list_events(base_url, token, listener_id, provider)
        item = find_event(events, marker)
        if item:
            return item
        time.sleep(0.5)
    raise RuntimeError(f"timed out waiting for event marker {marker} on listener {listener_id}")


def resolve_llm_provider():
    candidates = [
        ("groq", os.environ.get("GROQ_API_KEY", ""), os.environ.get("GROQ_BASE_URL", ""), os.environ.get("GROQ_MODEL", "")),
        ("openrouter", os.environ.get("OPENROUTER_API_KEY", ""), os.environ.get("OPENROUTER_BASE_URL", ""), os.environ.get("OPENROUTER_MODEL", "")),
        ("cerebras", os.environ.get("CEREBRAS_API_KEY", ""), os.environ.get("CEREBRAS_BASE_URL", ""), os.environ.get("CEREBRAS_MODEL", "")),
    ]
    for provider, key, base_url, model in candidates:
        if key.strip():
            return provider, key.strip(), base_url.strip(), model.strip()
    return None


def main():
    load_env_files()
    parser = argparse.ArgumentParser(description="API-only listener forwarding e2e")
    parser.add_argument("--base-url", default=os.environ.get("BASE_URL", "http://127.0.0.1:8131"))
    parser.add_argument("--token", default=os.environ.get("AGENTHOOK_API_TOKEN", ""))
    parser.add_argument("--email", default=f"listener-forward-{int(time.time())}@agentmail.to")
    parser.add_argument("--provider", default="generic-json")
    args = parser.parse_args()

    token = args.token.strip()
    account = None
    if not token:
        token, account = register_account(args.base_url, args.email)
    test_run_id = f"relay-{int(time.time() * 1000)}"

    sink = create_listener(args.base_url, token, args.provider, f"sink-{test_run_id}", f"sink{test_run_id}", plain_text_action="no_action", use_llm_fallback=False)
    target_key = f"relay_target_{test_run_id}"
    create_forward_target(args.base_url, token, target_key, sink["webhook_url"], f"sink-{test_run_id}")

    deterministic_source = create_listener(args.base_url, token, args.provider, f"srcdet-{test_run_id}", f"srcdet{test_run_id}", use_llm_fallback=False)
    create_skill(args.base_url, token, deterministic_source["type_key"], "det_forward_skill", "deterministic-forward-e2e", "forward_http", "")
    det_marker = f"{test_run_id}-det"
    det_payload = {"test_run_id": det_marker, "message": "deterministic-forward-e2e payload"}
    det_response = post_webhook(deterministic_source["webhook_url"], det_payload)
    det_source_event = poll_for_event(args.base_url, token, deterministic_source["listener_id"], args.provider, det_marker)
    det_sink_event = poll_for_event(args.base_url, token, sink["listener_id"], args.provider, det_marker)

    llm_provider = resolve_llm_provider()
    if not llm_provider:
        raise RuntimeError("No GROQ_API_KEY, OPENROUTER_API_KEY, or CEREBRAS_API_KEY available for LLM scenario")
    provider_name, api_key, base_url_override, model = llm_provider
    upsert_byok(args.base_url, token, provider_name, api_key, base_url_override, model)

    llm_source = create_listener(args.base_url, token, args.provider, f"srcllm-{test_run_id}", f"srcllm{test_run_id}", use_llm_fallback=True)
    create_skill(
        args.base_url,
        token,
        llm_source["type_key"],
        "llm_forward_skill",
        "llm-forward-e2e",
        "",
        f"When this matches, choose candidate_action forward_http and integration_target_key {target_key}.",
    )
    llm_marker = f"{test_run_id}-llm"
    llm_payload = {"test_run_id": llm_marker, "message": "llm-forward-e2e payload"}
    llm_response = post_webhook(llm_source["webhook_url"], llm_payload)
    llm_source_event = poll_for_event(args.base_url, token, llm_source["listener_id"], args.provider, llm_marker, timeout_sec=30)
    llm_sink_event = poll_for_event(args.base_url, token, sink["listener_id"], args.provider, llm_marker, timeout_sec=30)

    result = {
        "account": account,
        "base_url": args.base_url,
        "deterministic": {
            "source_listener": deterministic_source["listener_id"],
            "sink_listener": sink["listener_id"],
            "response_action": det_response.get("decision", {}).get("action_name"),
            "source_event_id": det_source_event.get("event_id"),
            "source_action": det_source_event.get("action_selected"),
            "sink_event_id": det_sink_event.get("event_id"),
        },
        "llm": {
            "provider": provider_name,
            "source_listener": llm_source["listener_id"],
            "sink_listener": sink["listener_id"],
            "response_action": llm_response.get("decision", {}).get("action_name"),
            "source_event_id": llm_source_event.get("event_id"),
            "source_action": llm_source_event.get("action_selected"),
            "sink_event_id": llm_sink_event.get("event_id"),
        },
    }

    if result["deterministic"]["response_action"] != "forward_http":
        raise RuntimeError(f"deterministic scenario did not return forward_http: {result['deterministic']}")
    if result["llm"]["response_action"] != "forward_http":
        raise RuntimeError(f"llm scenario did not return forward_http: {result['llm']}")
    if result["deterministic"]["source_event_id"] == result["deterministic"]["sink_event_id"]:
        raise RuntimeError("deterministic sink event id should differ from source event id")
    if result["llm"]["source_event_id"] == result["llm"]["sink_event_id"]:
        raise RuntimeError("llm sink event id should differ from source event id")

    print(json.dumps(result, indent=2))


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(str(exc), file=sys.stderr)
        sys.exit(1)
