#!/usr/bin/env python3
import json
import os
import signal
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path
from typing import Dict, Optional, Tuple


ROOT = Path(__file__).resolve().parents[1]
ENV_CANDIDATES = [ROOT / "local.env", ROOT / ".env"]
BASE_URL = os.environ.get("BASE_URL", "http://127.0.0.1:8092")
PORT = urllib.parse.urlparse(BASE_URL).port or 8092


def load_env_file(path: Path) -> Dict[str, str]:
    env: Dict[str, str] = {}
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        env[key.strip()] = value.strip()
    return env


def load_local_env() -> Dict[str, str]:
    merged: Dict[str, str] = {}
    for path in reversed(ENV_CANDIDATES):
        if path.exists():
            merged.update(load_env_file(path))
    return merged


def http_json(method: str, url: str, token: Optional[str] = None, body: Optional[dict] = None) -> dict:
    data = None if body is None else json.dumps(body).encode()
    headers = {"Accept": "application/json"}
    if body is not None:
        headers["Content-Type"] = "application/json"
    if token:
        headers["Authorization"] = f"Bearer {token}"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.loads(resp.read().decode())


def wait_for_healthz() -> None:
    deadline = time.time() + 30
    while time.time() < deadline:
        try:
            payload = http_json("GET", f"{BASE_URL}/healthz")
            if payload.get("ok") is True:
                return
        except Exception:
            time.sleep(0.5)
    raise RuntimeError("local server did not become healthy in time")


def resolve_provider_config(env_file: Dict[str, str], provider: str) -> Tuple[str, str, str]:
    provider = provider.strip().lower()
    if provider == "groq":
        return (
            env_file.get("GROQ_API_KEY", "").strip(),
            os.environ.get("LOCAL_GROQ_BASE_URL", "https://api.groq.com/openai/v1").strip(),
            os.environ.get("LOCAL_GROQ_MODEL", "llama-3.3-70b-versatile").strip(),
        )
    if provider == "cerebras":
        return (
            env_file.get("CEREBRAS_API_KEY", "").strip(),
            os.environ.get("LOCAL_CEREBRAS_BASE_URL", "https://api.cerebras.ai/v1").strip(),
            os.environ.get("LOCAL_CEREBRAS_MODEL", "llama-3.3-70b").strip(),
        )
    if provider == "openrouter":
        return (
            env_file.get("OPENROUTER_API_KEY", "").strip(),
            os.environ.get("LOCAL_OPENROUTER_BASE_URL", "https://openrouter.ai/api/v1").strip(),
            os.environ.get("LOCAL_OPENROUTER_MODEL", "openrouter/free").strip(),
        )
    raise RuntimeError(f"unsupported provider={provider}")


def main() -> int:
    env_file = load_local_env()
    provider = os.environ.get("LOCAL_LLM_PROVIDER", "groq").strip().lower()
    api_key, base_url, model = resolve_provider_config(env_file, provider)
    model = os.environ.get("LOCAL_LLM_MODEL", model).strip()
    base_url = os.environ.get("LOCAL_LLM_BASE_URL", base_url).strip()
    if not api_key:
        raise RuntimeError(f"missing API key for provider={provider}")
    fallback_providers = [p.strip().lower() for p in os.environ.get("LOCAL_LLM_FALLBACK_PROVIDERS", "").split(",") if p.strip()]
    force_primary_failure = os.environ.get("LOCAL_FORCE_PRIMARY_FAILURE", "").strip().lower() in {"1", "true", "yes"}

    env = os.environ.copy()
    env.update(env_file)
    env.update(
        {
            "PORT": str(PORT),
            "USE_IN_MEMORY_STORE": "true",
            "LLM_PROVIDER": provider,
            "LLM_API_KEY": api_key,
            "LLM_BASE_URL": base_url,
            "LLM_MODEL": model,
            "PINECONE_API_KEY": "",
            "PINECONE_INDEX_URL": "",
            "PINECONE_NAMESPACE": "default",
            "PUBLIC_BASE_URL": BASE_URL,
        }
    )

    log_file = tempfile.NamedTemporaryFile(prefix="agenthook-local-e2e-", suffix=".log", delete=False)
    log_file_path = Path(log_file.name)
    log_file.close()
    server = subprocess.Popen(
        ["go", "run", "./cmd/server"],
        cwd=ROOT,
        env=env,
        stdout=open(log_file_path, "w"),
        stderr=subprocess.STDOUT,
        text=True,
    )
    try:
        print(f"starting local server on {BASE_URL} with provider={provider} model={model}")
        wait_for_healthz()
        print("healthz ok")
        email = f"local-{provider}-{uuid.uuid4().hex[:8]}@agentmail.to"
        register = http_json("POST", f"{BASE_URL}/api/register/email", body={"email": email})
        token = register["token"]
        account = register["account"]
        print(f"registered account slug={account['slug']}")
        webhook_type = http_json(
            "POST",
            f"{BASE_URL}/api/webhooks/types",
            token=token,
            body={"type_key": "generic-json", "plain_text_action": "", "use_llm_fallback": True},
        )
        print(f"created webhook type={webhook_type['type_key']}")
        secret = http_json(
            "POST",
            f"{BASE_URL}/api/webhooks/secrets",
            token=token,
            body={"type_key": webhook_type["type_key"]},
        )
        print("created webhook secret")
        http_json(
            "POST",
            f"{BASE_URL}/v1/byok/providers",
            token=token,
            body={
                "provider": provider,
                "api_key": "invalid-test-key" if force_primary_failure else api_key,
                "base_url": base_url,
                "model": model,
                "is_default": True,
            },
        )
        print("configured byok provider")
        for fallback_provider in fallback_providers:
            fallback_key, fallback_base_url, fallback_model = resolve_provider_config(env_file, fallback_provider)
            if not fallback_key:
                raise RuntimeError(f"missing API key for fallback provider={fallback_provider}")
            http_json(
                "POST",
                f"{BASE_URL}/v1/byok/providers",
                token=token,
                body={
                    "provider": fallback_provider,
                    "api_key": fallback_key,
                    "base_url": fallback_base_url,
                    "model": fallback_model,
                    "is_default": False,
                },
            )
        if fallback_providers:
            print(f"configured fallback providers={','.join(fallback_providers)}")
        http_json(
            "POST",
            f"{BASE_URL}/api/policy/skills",
            token=token,
            body={
                "type_key": "generic-json",
                "skill_key": "workflow-summary",
                "skill_prompt": "Summarize the event clearly and return at least one useful tag.",
                "match_contains": "workflow,deploy,incident",
                "forced_action": "store_mysql",
                "memory_write_mode": "update_or_insert",
                "priority": 1,
                "enabled": True,
            },
        )
        print("created skill")
        big_payload = {
            "workflow": "Deploy AWS Lambda",
            "status": "completed",
            "conclusion": "success",
            "repository": {"full_name": "abhinaviitg18/webhook_listener"},
            "details": "signal " * 1200,
        }
        ingest_url = f"{BASE_URL}/url/{account['slug']}/generic-json/{secret['secret_value']}"
        http_json("POST", ingest_url, body=big_payload)
        print("ingested payload")
        events = http_json("GET", f"{BASE_URL}/api/events?limit=5", token=token)
        event = events[0]
        print(f"re-running event {event['id']}")
        rerun = http_json("POST", f"{BASE_URL}/api/events/{event['id']}/re-run", token=token)
        persisted = http_json("GET", f"{BASE_URL}/api/events/{event['id']}", token=token)
        print(
            json.dumps(
                {
                    "provider": provider,
                    "model": model,
                    "fallback_providers": fallback_providers,
                    "event_id": event["id"],
                    "rerun_decision": rerun.get("decision"),
                    "response_processed_text_len": len((rerun.get("event") or {}).get("processed_text") or ""),
                    "persisted_processed_text_len": len(persisted.get("processed_text") or ""),
                    "persisted_tags_json": persisted.get("tags_json"),
                },
                indent=2,
            )
        )
        if len(persisted.get("processed_text") or "") == 0:
            return 2
        return 0
    except urllib.error.HTTPError as exc:
        body = exc.read().decode()
        print(json.dumps({"error": f"http {exc.code}", "body": body[:800]}, indent=2))
        return 1
    except Exception as exc:
        print(json.dumps({"error": str(exc)}, indent=2))
        return 1
    finally:
        if server.poll() is None:
            server.send_signal(signal.SIGINT)
            try:
                server.wait(timeout=10)
            except subprocess.TimeoutExpired:
                server.kill()
        if log_file_path.exists():
            output = log_file_path.read_text()
            if output:
                print("\n--- local server log tail ---")
                print(output[-4000:])


if __name__ == "__main__":
    sys.exit(main())
