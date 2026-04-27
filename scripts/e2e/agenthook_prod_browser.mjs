#!/usr/bin/env node

import fs from "node:fs";
import path from "node:path";
import process from "node:process";
import { chromium } from "playwright";

function parseArgs(argv) {
  const out = { _: [] };
  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    if (!arg.startsWith("--")) {
      out._.push(arg);
      continue;
    }
    const key = arg.slice(2);
    const next = argv[i + 1];
    if (!next || next.startsWith("--")) {
      out[key] = "true";
      continue;
    }
    out[key] = next;
    i += 1;
  }
  return out;
}

function requireArg(args, key) {
  const value = args[key];
  if (!value) {
    throw new Error(`missing required --${key}`);
  }
  return value;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function ensureDir(dir) {
  fs.mkdirSync(dir, { recursive: true });
}

function resolveAgentmailApiKey() {
  if (process.env.AGENTMAIL_API_KEY) {
    return process.env.AGENTMAIL_API_KEY;
  }
  const envPath = path.resolve(process.cwd(), ".env");
  if (!fs.existsSync(envPath)) {
    throw new Error("AGENTMAIL_API_KEY missing and .env not found");
  }
  const envText = fs.readFileSync(envPath, "utf8");
  const match = envText.match(/^AGENTMAIL_API_KEY=(.+)$/m);
  if (!match) {
    throw new Error("AGENTMAIL_API_KEY missing from environment and .env");
  }
  return match[1].trim();
}

async function fetchOtp(email, challengeStartedAt, attempts = 12, delayMs = 5_000) {
  const apiKey = resolveAgentmailApiKey();
  for (let i = 0; i < attempts; i += 1) {
    const response = await fetch(`https://api.agentmail.to/v0/inboxes/${encodeURIComponent(email)}/messages`, {
      headers: {
        Authorization: `Bearer ${apiKey}`,
      },
    });
    if (!response.ok) {
      throw new Error(`AgentMail message fetch failed: ${response.status}`);
    }
    const payload = await response.json();
    const messages = Array.isArray(payload.messages) ? payload.messages : [];
    for (const message of messages) {
      const fromValue = Array.isArray(message.from) ? message.from.join(", ") : String(message.from || "");
      const subject = String(message.subject || "");
      const haystack = `${fromValue} ${subject}`.toLowerCase();
      if (!haystack.includes("scalekit") && !haystack.includes("hookweb")) {
        continue;
      }
      const timestamp = new Date(message.timestamp || 0);
      if (!(timestamp instanceof Date) || Number.isNaN(timestamp.valueOf())) {
        continue;
      }
      if (timestamp <= challengeStartedAt) {
        continue;
      }
      const match = subject.match(/(\d{6})/);
      if (match) {
        return match[1];
      }
    }
    await sleep(delayMs);
  }
  throw new Error("could not find a fresh ScaleKit OTP after the current login attempt");
}

async function triggerLoginChallenge(page, email) {
  const baseUrl = process.env.BASE_URL || "https://app.agenthook.store";
  await page.goto(`${baseUrl}/auth/scalekit/login`, {
    waitUntil: "domcontentloaded",
  });
  await page.waitForLoadState("networkidle");

  const emailInput = page.locator('input[type="email"], input[name="email"]').first();
  await emailInput.waitFor({ state: "visible", timeout: 30_000 });
  await emailInput.fill(email);

  const submitButton = page.getByRole("button").filter({
    hasText: /continue|send|login|sign in/i,
  }).first();
  const challengeStartedAt = new Date();
  await submitButton.click();
  return challengeStartedAt;
}

async function completeOtp(page, code) {
  const otpInputs = page.locator('input[inputmode="numeric"], input[autocomplete*="one-time-code"], input[name*="otp"], input[name*="code"]');
  const otpCount = await otpInputs.count();
  if (otpCount >= 6) {
    for (let i = 0; i < 6; i += 1) {
      await otpInputs.nth(i).fill(code[i]);
    }
  } else {
    const single = otpInputs.first();
    await single.waitFor({ state: "visible", timeout: 30_000 });
    await single.fill(code);
  }

  const verifyButton = page.getByRole("button").filter({
    hasText: /verify|continue|submit|login|sign in/i,
  }).first();
  if (await verifyButton.isVisible().catch(() => false)) {
    await verifyButton.click();
  }
}

async function waitForApp(page) {
  await page.waitForURL(/\/app(?:\/)?(?:\?|#|$)/, { timeout: 60_000 });
  await page.waitForLoadState("networkidle");
  await page.locator("text=Storyboard").first().waitFor({ state: "visible", timeout: 60_000 });
}

async function mintApiToken(page) {
  const response = await page.evaluate(async () => {
    const res = await fetch("/v1/auth/tokens", {
      method: "POST",
      credentials: "include",
      headers: { accept: "application/json" },
    });
    const text = await res.text();
    return {
      ok: res.ok,
      status: res.status,
      body: text,
    };
  });
  if (!response.ok) {
    throw new Error(`failed to mint API token: ${response.status} ${response.body}`);
  }
  const payload = JSON.parse(response.body);
  if (!payload.token) {
    throw new Error("mint API token response did not include token");
  }
  return payload.token;
}

async function loginAndMintToken(args) {
  const email = requireArg(args, "email");
  const outDir = requireArg(args, "out-dir");
  ensureDir(outDir);

  const browser = await chromium.launch({
    headless: args.headless !== "false",
  });
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    const challengeStartedAt = await triggerLoginChallenge(page, email);
    await sleep(2_000);
    const code = await fetchOtp(email, challengeStartedAt);
    await completeOtp(page, code);
    await waitForApp(page);
    const token = await mintApiToken(page);
    const storagePath = path.join(outDir, "auth-storage.json");
    await context.storageState({ path: storagePath });
    const result = {
      token,
      storage_state_path: storagePath,
      current_url: page.url(),
    };
    const resultPath = path.join(outDir, "browser-login.json");
    fs.writeFileSync(resultPath, `${JSON.stringify(result, null, 2)}\n`);
    process.stdout.write(`${JSON.stringify(result)}\n`);
  } finally {
    await browser.close();
  }
}

async function verifyStoryboard(args) {
  const outDir = requireArg(args, "out-dir");
  const githubNeedle = requireArg(args, "github-needle");
  const agentmailNeedle = requireArg(args, "agentmail-needle");
  const storageStatePath = requireArg(args, "storage-state");
  const baseUrl = args["base-url"] || process.env.BASE_URL || "https://app.agenthook.store";
  ensureDir(outDir);

  const browser = await chromium.launch({
    headless: args.headless !== "false",
  });
  const context = await browser.newContext({ storageState: storageStatePath });
  const page = await context.newPage();

  try {
    await page.goto(`${baseUrl}/app`, { waitUntil: "domcontentloaded" });
    await page.waitForLoadState("networkidle");
    await page.locator("text=Storyboard").first().waitFor({ state: "visible", timeout: 60_000 });

    await page.waitForFunction(
      ([githubText, mailText]) => {
        const body = document.body.innerText || "";
        return body.includes(githubText) && body.includes(mailText);
      },
      [githubNeedle, agentmailNeedle],
      { timeout: 90_000 }
    );

    const screenshotPath = path.join(outDir, "storyboard-verification.png");
    await page.screenshot({ path: screenshotPath, fullPage: true });
    const result = {
      screenshot_path: screenshotPath,
      current_url: page.url(),
      github_needle: githubNeedle,
      agentmail_needle: agentmailNeedle,
    };
    const resultPath = path.join(outDir, "storyboard-verification.json");
    fs.writeFileSync(resultPath, `${JSON.stringify(result, null, 2)}\n`);
    process.stdout.write(`${JSON.stringify(result)}\n`);
  } finally {
    await browser.close();
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  const mode = args._[0];
  if (mode === "login-and-mint-token") {
    await loginAndMintToken(args);
    return;
  }
  if (mode === "verify-storyboard") {
    await verifyStoryboard(args);
    return;
  }
  throw new Error("expected mode: login-and-mint-token | verify-storyboard");
}

main().catch((error) => {
  console.error(error.message || error);
  process.exit(1);
});
