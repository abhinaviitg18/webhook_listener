import React, { useEffect, useMemo, useState } from 'react';
import { TopAppBar } from './components/TopAppBar';
import { BottomNavBar } from './components/BottomNavBar';
import { Metrics } from './components/Metrics';
import { StoryboardCard } from './components/StoryboardCard';
import {
  Plus,
  RefreshCw,
  Copy,
  Check,
  Brain,
  LogIn,
  Link2,
  KeyRound,
  Wand2,
  TestTube2,
  Sparkles,
  ChevronDown,
  ChevronUp,
  Cable,
  MessageSquareQuote,
  Mail,
  BadgeCheck,
  Save,
  Trash2,
} from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';
import { useAuth } from './context/AuthContext';

const VALID_TABS = new Set(['storyboard', 'skills', 'integrations', 'urls', 'byok']);

const PROVIDER_OPTIONS = [
  'github',
  'stripe',
  'slack',
  'resend',
  'shopify',
  'openai',
  'generic-json',
];

const MEMORY_WRITE_MODES = ['update_or_insert', 'insert_only', 'disabled'];

const FORCED_ACTION_OPTIONS = ['store_mysql', 'no_action', 'manual_review', 'forward_http', 'forward_telegram', 'slack_notify', 'crm_upsert', 'ticket_create'];
const INTEGRATION_TARGET_TYPES = ['http', 'telegram', 'openclaw', 'custom'];
const INTEGRATION_ACTION_OPTIONS = ['forward_http', 'forward_telegram', 'slack_notify', 'crm_upsert', 'ticket_create'];
const INTEGRATION_AUTH_TYPES = ['none', 'bearer_header', 'custom_header', 'query_param'];

const INTEGRATION_PRESETS = {
  openclaw: {
    target_key: 'openclaw_primary',
    target_type: 'http',
    purpose: 'Forward structured leads and tickets into OpenClaw.',
    enabled: true,
    allowed_actions: ['forward_http', 'crm_upsert', 'ticket_create'],
    config: {
      url: 'https://api.openclaw.example/v1/intake',
      method: 'POST',
    },
    auth: {
      type: 'bearer_header',
      secret_ref: 'openclaw_api_key',
      header_name: 'Authorization',
      prefix: 'Bearer ',
      env_var: 'OPENCLAW_API_KEY',
    },
    schema: {
      entity_payload: 'object',
      source: 'agenthook',
    },
  },
  forward_url: {
    target_key: 'forward_url_primary',
    target_type: 'http',
    purpose: 'Forward events to any custom HTTP endpoint.',
    enabled: true,
    allowed_actions: ['forward_http', 'slack_notify'],
    config: {
      url: 'https://example.com/webhook',
      method: 'POST',
      headers: {
        'x-agenthook-source': 'listener',
      },
    },
    auth: {
      type: 'none',
      secret_ref: '',
      header_name: '',
      prefix: '',
      query_param: '',
      env_var: '',
    },
    schema: {},
  },
};

const SKILL_PACKS = {
  whatsapp: {
    label: 'WhatsApp Pack',
    specificMatchContains: 'whatsapp',
    skills: [
      {
        skill_key: 'channel_whatsapp_router',
        skill_prompt: 'Route conversational WhatsApp messages into lead capture, support escalation, or spam filtering.',
        match_contains: 'whatsapp,message,phone,chat',
        forced_action: 'store_mysql',
        memory_write_mode: 'update_or_insert',
        priority: 10,
        enabled: true,
      },
      {
        skill_key: 'whatsapp_spam_filter',
        skill_prompt: 'Detect promotional or low-value WhatsApp outreach and suppress it.',
        match_contains: 'promo,offer,discount,click here,unsubscribe',
        forced_action: 'no_action',
        memory_write_mode: 'disabled',
        priority: 20,
        enabled: true,
      },
      {
        skill_key: 'whatsapp_lead_capture',
        skill_prompt: 'Extract lead name, company, and intent from WhatsApp messages and prepare CRM-ready output.',
        match_contains: 'demo,pricing,interested,company,trial',
        forced_action: 'crm_upsert',
        memory_write_mode: 'update_or_insert',
        priority: 30,
        enabled: true,
      },
      {
        skill_key: 'whatsapp_support_escalation',
        skill_prompt: 'Summarize urgent support issues from WhatsApp and notify the operations team.',
        match_contains: 'urgent,error,broken,down,issue,help',
        forced_action: 'slack_notify',
        memory_write_mode: 'update_or_insert',
        priority: 40,
        enabled: true,
      },
    ],
  },
  email: {
    label: 'Email Pack',
    specificMatchContains: 'email',
    skills: [
      {
        skill_key: 'channel_email_router',
        skill_prompt: 'Route email-style traffic into marketing noise, sales leads, or finance approvals.',
        match_contains: 'subject,from,to,email,inbox',
        forced_action: 'store_mysql',
        memory_write_mode: 'update_or_insert',
        priority: 10,
        enabled: true,
      },
      {
        skill_key: 'email_marketing_noise_filter',
        skill_prompt: 'Suppress newsletters and low-signal email campaigns.',
        match_contains: 'newsletter,unsubscribe,discount,offer,promo',
        forced_action: 'no_action',
        memory_write_mode: 'disabled',
        priority: 20,
        enabled: true,
      },
      {
        skill_key: 'email_sales_lead_router',
        skill_prompt: 'Extract qualified inbound lead details from email and prepare a CRM upsert.',
        match_contains: 'demo,enterprise,pricing,quote,trial',
        forced_action: 'crm_upsert',
        memory_write_mode: 'update_or_insert',
        priority: 30,
        enabled: true,
      },
      {
        skill_key: 'email_invoice_approval',
        skill_prompt: 'Detect invoice and approval emails that need finance follow-up or ticketing.',
        match_contains: 'invoice,approval,payment,approve,overdue',
        forced_action: 'ticket_create',
        memory_write_mode: 'update_or_insert',
        priority: 40,
        enabled: true,
      },
    ],
  },
  gate: {
    label: 'GetApproval Pack',
    specificMatchContains: 'approval',
    skills: [
      {
        skill_key: 'channel_getapproval_router',
        skill_prompt: 'Route approval workflow events into request, escalation, or archive behavior.',
        match_contains: 'approval,approver,pending,request_status,approval_url',
        forced_action: 'store_mysql',
        memory_write_mode: 'update_or_insert',
        priority: 10,
        enabled: true,
      },
      {
        skill_key: 'approval_request_classifier',
        skill_prompt: 'Summarize standard approval requests and capture who needs to decide and by when.',
        match_contains: 'approval requested,pending approval,requester',
        forced_action: 'manual_review',
        memory_write_mode: 'update_or_insert',
        priority: 20,
        enabled: true,
      },
      {
        skill_key: 'approval_urgent_escalation',
        skill_prompt: 'Escalate urgent approval messages to ops or finance immediately.',
        match_contains: 'urgent approval,blocked,release,critical',
        forced_action: 'slack_notify',
        memory_write_mode: 'update_or_insert',
        priority: 30,
        enabled: true,
      },
      {
        skill_key: 'approval_auto_archive',
        skill_prompt: 'Track completed approval events without triggering downstream action.',
        match_contains: 'approved,rejected,completed',
        forced_action: 'store_mysql',
        memory_write_mode: 'insert_only',
        priority: 40,
        enabled: true,
      },
    ],
  },
};

const HOMEPAGE_SUPPORT_POINTS = [
  'Works with any app that can send HTTP requests',
  'Filter noise before it reaches OpenClaw, Slack, CRMs, or your own APIs',
  'Reclassify old events when rules or skills improve',
];

const HOMEPAGE_PAIN_POINTS = [
  'Too many webhooks are technically valid but operationally useless.',
  'Downstream tools get flooded with heartbeats, retries, status pings, and marketing noise.',
  'Expensive systems should not be invoked for every event.',
  'Rules change over time, but most webhook pipelines are hard to correct after the fact.',
  'Teams end up building brittle glue code just to decide what is worth storing, routing, or escalating.',
];

const HOMEPAGE_FLOW = [
  {
    title: 'Receive',
    body: 'Any app can send JSON to AgentHook using a listener URL or API token.',
  },
  {
    title: 'Understand',
    body: 'AgentHook applies transforms, routing logic, skills, and optional LLM classification.',
  },
  {
    title: 'Filter',
    body: 'It suppresses noise like heartbeats, low-value health checks, and repetitive status events.',
  },
  {
    title: 'Forward or store',
    body: 'It sends meaningful events to OpenClaw, your CRM, Slack, Telegram, or any forward URL.',
  },
];

const HOMEPAGE_EXAMPLES = [
  {
    title: 'OpenClaw intake',
    body: 'Only forward qualified leads, urgent tickets, or approval events into OpenClaw. Skip heartbeats and low-signal noise.',
  },
  {
    title: 'WhatsApp lead routing',
    body: 'Capture demo requests, pricing intent, and support escalations from WhatsApp-style messages.',
  },
  {
    title: 'Email triage',
    body: 'Suppress newsletters, route enterprise leads to CRM, and escalate invoice approvals or critical replies.',
  },
  {
    title: 'Approval workflows',
    body: 'Detect urgent approval blockers and notify the right team instead of treating every update the same way.',
  },
  {
    title: 'Generic webhook cleanup',
    body: 'Accept raw events from internal systems, summarize them, tag them, and forward only the important ones.',
  },
];

const HOMEPAGE_VALUE_POINTS = [
  'Cuts downstream automation cost',
  'Reduces alert fatigue and webhook noise',
  'Centralizes routing logic instead of scattering it across scripts',
  'Lets you evolve classification over time with reclassify',
  'Works with your existing tools instead of replacing them',
];

function safeJSONParse(text, fallback = null) {
  try {
    return text ? JSON.parse(text) : fallback;
  } catch {
    return fallback;
  }
}

async function apiRequest(path, options = {}) {
  const response = await fetch(path, {
    credentials: 'include',
    headers: {
      Accept: 'application/json',
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      ...(options.headers || {}),
    },
    ...options,
  });

  const text = await response.text();
  const data = safeJSONParse(text, null);
  if (!response.ok) {
    const errorMessage = data?.error || text || `Request failed with status ${response.status}`;
    throw new Error(errorMessage);
  }
  return data;
}

function prettyJSON(value) {
  return JSON.stringify(value, null, 2);
}

function parseJSONText(text, fallback) {
  if (typeof text !== 'string' || text.trim() === '') {
    return fallback;
  }
  const parsed = safeJSONParse(text, fallback);
  return parsed ?? fallback;
}

function arrayFromJSONText(text) {
  const parsed = parseJSONText(text, []);
  return Array.isArray(parsed) ? parsed : [];
}

function objectFromJSONText(text) {
  const parsed = parseJSONText(text, {});
  return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
}

function targetConfigFromRecord(target) {
  const parsed = objectFromJSONText(target?.config_json);
  if (parsed.config && typeof parsed.config === 'object' && !Array.isArray(parsed.config)) {
    return parsed.config;
  }
  return parsed;
}

function targetRecordDetails(target) {
  return objectFromJSONText(target?.config_json);
}

function parseObjectOrThrow(text, label) {
  if (typeof text !== 'string' || text.trim() === '') {
    return {};
  }
  const parsed = safeJSONParse(text, undefined);
  if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw new Error(`${label} must be valid JSON object`);
  }
  return parsed;
}

function payloadPreview(event) {
  const candidates = [event?.processed_text, event?.raw_payload_json, event?.payload_json];
  for (const candidate of candidates) {
    if (typeof candidate !== 'string' || candidate.trim() === '') continue;
    const parsed = safeJSONParse(candidate, null);
    if (parsed) {
      return prettyJSON(parsed);
    }
    return candidate.trim();
  }
  return '';
}

function inferTypeKey(listener) {
  if (!listener) return '';
  return listener.type_key || '';
}

function listenerIngressTemplate(listener, publicAlias) {
  if (!listener) return `https://app.agenthook.store/${publicAlias}.[secret]`;
  return listener.webhook_url_template || `https://app.agenthook.store/${publicAlias}.[secret]`;
}

function listenerWebhookIDTemplate(listener, publicAlias) {
  if (!listener) return `${publicAlias}.[secret]@app.agenthook.store`;
  return listener.webhook_id_template || `${publicAlias}.[secret]@app.agenthook.store`;
}

function Panel({ title, subtitle, action, children }) {
  return (
    <section className="glass-card border border-slate-800 rounded-2xl p-4 space-y-4">
      <div className="flex items-start justify-between gap-3">
        <div>
          <h3 className="text-white font-semibold">{title}</h3>
          {subtitle && <p className="text-slate-500 text-xs mt-1">{subtitle}</p>}
        </div>
        {action}
      </div>
      {children}
    </section>
  );
}

function FormField({ label, children, hint }) {
  return (
    <label className="space-y-1 block">
      <span className="text-[10px] text-slate-500 font-label-caps">{label}</span>
      {children}
      {hint && <span className="block text-[11px] text-slate-500">{hint}</span>}
    </label>
  );
}

function TextInput(props) {
  return (
    <input
      {...props}
      className={`w-full bg-slate-900 border border-slate-800 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-1 focus:ring-primary ${props.className || ''}`}
    />
  );
}

function TextArea(props) {
  return (
    <textarea
      {...props}
      className={`w-full min-h-28 bg-slate-900 border border-slate-800 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-1 focus:ring-primary ${props.className || ''}`}
    />
  );
}

function Select(props) {
  return (
    <select
      {...props}
      className={`w-full bg-slate-900 border border-slate-800 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-1 focus:ring-primary ${props.className || ''}`}
    />
  );
}

function InlineNotice({ tone = 'info', children }) {
  const classes = {
    info: 'bg-slate-900/70 border-slate-800 text-slate-300',
    success: 'bg-green-500/10 border-green-500/20 text-green-300',
    error: 'bg-red-500/10 border-red-500/20 text-red-300',
  };
  return (
    <div className={`rounded-xl border px-3 py-2 text-xs ${classes[tone] || classes.info}`}>
      {children}
    </div>
  );
}

function CopyButton({ value, copiedKey, setCopiedKey, copyKey, title = 'Copy' }) {
  return (
    <button
      onClick={async () => {
        await navigator.clipboard.writeText(value);
        setCopiedKey(copyKey);
        setTimeout(() => setCopiedKey(''), 1200);
      }}
      className="text-slate-500 hover:text-white"
      title={title}
    >
      {copiedKey === copyKey ? <Check size={14} className="text-green-500" /> : <Copy size={14} />}
    </button>
  );
}

function LandingSection({ id, eyebrow, title, children, className = '' }) {
  return (
    <section id={id} className={`space-y-4 ${className}`}>
      {eyebrow && <p className="text-[10px] uppercase tracking-[0.24em] text-indigo-400 font-label-caps">{eyebrow}</p>}
      <div className="space-y-2">
        <h2 className="text-2xl md:text-3xl font-h1 text-white">{title}</h2>
        {children}
      </div>
    </section>
  );
}

function MarketingHome({ login, error }) {
  const scrollToExamples = () => {
    document.getElementById('examples')?.scrollIntoView({ behavior: 'smooth', block: 'start' });
  };

  return (
    <div className="min-h-screen bg-surface text-on-surface">
      <div className="relative overflow-hidden border-b border-slate-800 bg-[radial-gradient(circle_at_top,_rgba(99,102,241,0.18),_transparent_45%),linear-gradient(180deg,_rgba(15,23,42,0.94),_rgba(2,6,23,1))]">
        <div className="absolute inset-0 bg-[linear-gradient(120deg,transparent_0%,rgba(59,130,246,0.06)_32%,transparent_70%)]" />
        <div className="relative max-w-6xl mx-auto px-6 py-8 md:py-10">
          <div className="flex items-center justify-between gap-4 mb-14">
            <div>
              <p className="text-lg font-bold tracking-tight text-white font-h1">AgentHook</p>
              <p className="text-xs uppercase tracking-[0.22em] text-slate-500">Webhook routing for operators</p>
            </div>
            <button
              onClick={login}
              className="inline-flex items-center gap-2 border border-slate-700 bg-slate-900/70 px-4 py-2 rounded-xl text-sm font-semibold text-white hover:bg-slate-800 transition-colors"
            >
              <LogIn size={16} />
              Sign in
            </button>
          </div>

          <div className="grid gap-10 lg:grid-cols-[1.2fr_0.8fr] lg:items-center">
            <div className="space-y-8">
              <div className="space-y-4">
                <p className="text-[10px] uppercase tracking-[0.28em] text-indigo-400 font-label-caps">Turn noisy webhooks into useful actions</p>
                <h1 className="text-4xl md:text-6xl font-h1 text-white leading-tight max-w-3xl">
                  Stop paying attention to every event just because your apps can send one.
                </h1>
                <p className="text-base md:text-lg text-slate-300 max-w-2xl">
                  AgentHook receives events from any app, filters out noise like heartbeats and routine status pings, classifies what matters, and forwards the right payload to the right tool.
                </p>
              </div>

              {error && (
                <div className="bg-red-500/10 border border-red-500/20 text-red-300 px-4 py-3 rounded-xl text-sm">
                  Authentication failed: {error}
                </div>
              )}

              <div className="flex flex-col sm:flex-row gap-3">
                <button
                  onClick={login}
                  className="inline-flex items-center justify-center gap-2 bg-primary text-on-primary px-6 py-4 rounded-2xl font-bold active:scale-95 transition-transform"
                >
                  <LogIn size={18} />
                  Create listener
                </button>
                <button
                  onClick={scrollToExamples}
                  className="inline-flex items-center justify-center gap-2 border border-slate-700 bg-slate-950/50 px-6 py-4 rounded-2xl font-semibold text-white hover:bg-slate-900 transition-colors"
                >
                  View examples
                </button>
              </div>

              <div className="grid gap-3 sm:grid-cols-3">
                {HOMEPAGE_SUPPORT_POINTS.map((item) => (
                  <div key={item} className="rounded-2xl border border-slate-800 bg-slate-950/40 px-4 py-4 text-sm text-slate-300">
                    {item}
                  </div>
                ))}
              </div>
            </div>

            <div className="rounded-[28px] border border-slate-800 bg-slate-950/70 p-5 shadow-2xl shadow-indigo-950/20">
              <div className="flex items-center justify-between mb-4">
                <div>
                  <p className="text-white text-sm font-semibold">Operator preview</p>
                  <p className="text-[11px] text-slate-500">Filter, route, and reclassify from one place</p>
                </div>
                <span className="rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2 py-1 text-[10px] font-bold text-emerald-300">
                  LIVE FLOW
                </span>
              </div>
              <div className="space-y-3">
                <div className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4">
                  <p className="text-[10px] uppercase tracking-[0.2em] text-slate-500">Incoming</p>
                  <pre className="mt-2 whitespace-pre-wrap break-words text-[11px] font-code-snippet text-slate-300">{`{"event":"heartbeat","kind":"metrics","service":"openclaw","ok":true}`}</pre>
                </div>
                <div className="rounded-2xl border border-slate-800 bg-slate-900/70 p-4">
                  <p className="text-[10px] uppercase tracking-[0.2em] text-slate-500">Decision</p>
                  <p className="mt-2 text-sm text-slate-200">Dropped as low-value noise. No downstream run triggered.</p>
                </div>
                <div className="rounded-2xl border border-indigo-500/20 bg-indigo-500/10 p-4">
                  <p className="text-[10px] uppercase tracking-[0.2em] text-indigo-300">High-signal example</p>
                  <pre className="mt-2 whitespace-pre-wrap break-words text-[11px] font-code-snippet text-indigo-100">{`{"event":"lead.created","company":"Acme","message":"Interested in a demo"}`}</pre>
                  <p className="mt-2 text-sm text-indigo-100">Forwarded to OpenClaw, tagged as lead, retained in storyboard for reclassification later.</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <main className="max-w-6xl mx-auto px-6 py-16 space-y-20">
        <LandingSection eyebrow="Why teams need this" title="Most webhook infrastructure stops at delivery.">
          <p className="text-slate-300 max-w-3xl">
            The real problem starts after that: deciding what is spam, what is noise, what should be stored, and what should trigger action.
          </p>
          <div className="grid gap-3 md:grid-cols-2">
            {HOMEPAGE_PAIN_POINTS.map((item) => (
              <div key={item} className="rounded-2xl border border-slate-800 bg-slate-950/40 px-4 py-4 text-sm text-slate-300">
                {item}
              </div>
            ))}
          </div>
        </LandingSection>

        <LandingSection eyebrow="What AgentHook does" title="A webhook control layer for founders, ops, and developer teams.">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
            {HOMEPAGE_FLOW.map((step, index) => (
              <div key={step.title} className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-3">
                <div className="flex items-center justify-between">
                  <span className="text-[10px] uppercase tracking-[0.22em] text-slate-500">Step {index + 1}</span>
                  <span className="text-indigo-400 text-sm font-semibold">0{index + 1}</span>
                </div>
                <h3 className="text-white text-lg font-semibold">{step.title}</h3>
                <p className="text-sm text-slate-300">{step.body}</p>
              </div>
            ))}
          </div>
          <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5">
            <p className="text-sm text-slate-300">
              You also get a storyboard of past events, reclassification for historical messages, reusable skills and integrations, and support for both single-tenant and multitenant listener modes.
            </p>
          </div>
        </LandingSection>

        <LandingSection id="examples" eyebrow="Examples of what you can do" title="Use AgentHook in front of any noisy workflow.">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {HOMEPAGE_EXAMPLES.map((example) => (
              <div key={example.title} className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-2">
                <h3 className="text-white text-lg font-semibold">{example.title}</h3>
                <p className="text-sm text-slate-300">{example.body}</p>
              </div>
            ))}
          </div>
        </LandingSection>

        <LandingSection eyebrow="OpenClaw ROI" title="Reduce OpenClaw cost by sending only meaningful events.">
          <div className="grid gap-6 lg:grid-cols-[1fr_1fr]">
            <div className="rounded-3xl border border-red-500/10 bg-red-500/5 p-6 space-y-4">
              <p className="text-[10px] uppercase tracking-[0.22em] text-red-300">Without AgentHook</p>
              <h3 className="text-white text-xl font-semibold">Every event reaches downstream automation.</h3>
              <p className="text-sm text-slate-300">
                If OpenClaw or another downstream workflow engine checks every heartbeat, sync ping, or routine status update, you pay for noise.
              </p>
              <ul className="space-y-2 text-sm text-slate-300">
                <li>Heartbeat and health-check events still consume attention.</li>
                <li>Routine metrics and retries can trigger unnecessary runs.</li>
                <li>Teams pay to discover most events are irrelevant.</li>
              </ul>
            </div>
            <div className="rounded-3xl border border-emerald-500/20 bg-emerald-500/10 p-6 space-y-4">
              <p className="text-[10px] uppercase tracking-[0.22em] text-emerald-300">With AgentHook</p>
              <h3 className="text-white text-xl font-semibold">Only classified, useful events move forward.</h3>
              <p className="text-sm text-slate-300">
                Instead of letting OpenClaw inspect every event just to discover most of them are irrelevant, AgentHook does the cheap filtering first and forwards only the events that deserve automation.
              </p>
              <ul className="space-y-2 text-sm text-slate-300">
                <li>Heartbeats and health checks can be dropped or archived with no downstream action.</li>
                <li>Only high-signal events like qualified leads, support incidents, failed workflows, or urgent approvals move forward.</li>
                <li>That means lower processing cost, fewer false alerts, and cleaner operational queues.</li>
              </ul>
            </div>
          </div>
        </LandingSection>

        <LandingSection eyebrow="Any app can call AgentHook" title="If your app can make an HTTP request, it can send events here.">
          <p className="text-slate-300 max-w-3xl">
            Use a listener URL for inbound webhooks, or use API tokens for direct management and testing. AgentHook accepts JSON payloads and then applies routing, filtering, tagging, and forwarding logic.
          </p>
          <div className="grid gap-6 lg:grid-cols-[0.95fr_1.05fr]">
            <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5">
              <p className="text-[10px] uppercase tracking-[0.2em] text-slate-500">HTTP example</p>
                  <pre className="mt-3 overflow-auto rounded-2xl border border-slate-800 bg-slate-950 p-4 text-[12px] text-slate-200 font-code-snippet">{`POST /{userkey}.{secret}
Content-Type: application/json

{
  "event": "lead.created",
  "source": "website",
  "company": "Acme",
  "message": "Interested in a demo"
}`}</pre>
            </div>
            <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-4">
              <p className="text-sm text-slate-300">
                AgentHook can store it, classify it, tag it, suppress it, or forward it to an integration like OpenClaw or a custom URL.
              </p>
              <div className="grid gap-3 sm:grid-cols-2">
                {['Create listeners', 'Create integrations', 'Create secret refs', 'Reclassify historical messages'].map((item) => (
                  <div key={item} className="rounded-2xl border border-slate-800 bg-slate-900/60 px-4 py-4 text-sm text-slate-200">
                    {item}
                  </div>
                ))}
              </div>
            </div>
          </div>
        </LandingSection>

        <LandingSection eyebrow="Why teams use AgentHook" title="Practical automation without scattering glue code everywhere.">
          <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-5">
            {HOMEPAGE_VALUE_POINTS.map((item) => (
              <div key={item} className="rounded-2xl border border-slate-800 bg-slate-950/40 px-4 py-4 text-sm text-slate-300">
                {item}
              </div>
            ))}
          </div>
        </LandingSection>

        <section className="rounded-[32px] border border-indigo-500/20 bg-[linear-gradient(135deg,rgba(79,70,229,0.16),rgba(15,23,42,0.92))] px-6 py-8 md:px-8 md:py-10">
          <div className="max-w-3xl space-y-4">
            <p className="text-[10px] uppercase tracking-[0.24em] text-indigo-200 font-label-caps">Start with one noisy webhook</p>
            <h2 className="text-3xl md:text-4xl font-h1 text-white">Create a listener, send a sample payload, and decide what should be dropped, stored, or forwarded.</h2>
            <p className="text-slate-200">
              AgentHook is built to sit in front of the tools you already use, not replace them.
            </p>
            <div className="flex flex-col sm:flex-row gap-3">
              <button
                onClick={login}
                className="inline-flex items-center justify-center gap-2 bg-primary text-on-primary px-6 py-4 rounded-2xl font-bold active:scale-95 transition-transform"
              >
                <LogIn size={18} />
                Create listener
              </button>
              <button
                onClick={scrollToExamples}
                className="inline-flex items-center justify-center gap-2 border border-indigo-200/20 bg-slate-950/30 px-6 py-4 rounded-2xl font-semibold text-white hover:bg-slate-900/60 transition-colors"
              >
                View example integrations
              </button>
            </div>
          </div>
        </section>
      </main>
    </div>
  );
}

function App() {
  const { user, setUser, isAuthenticated, loading, error, login, logout } = useAuth();
  const tabParam = new URLSearchParams(window.location.search).get('tab');
  const [activeTab, setActiveTab] = useState(VALID_TABS.has(tabParam) ? tabParam : 'storyboard');
  const [copied, setCopied] = useState('');
  const [events, setEvents] = useState([]);
  const [listeners, setListeners] = useState([]);
  const [fetching, setFetching] = useState(false);
  const [activeTag, setActiveTag] = useState(null);
  const [reclassifyingEventIDs, setReclassifyingEventIDs] = useState({});

  const publicAlias = user?.public_alias || user?.slug || '[userkey]';
  const ingressTemplate = listeners.length > 0
    ? listenerIngressTemplate(listeners[0], publicAlias)
    : `https://app.agenthook.store/${publicAlias}.[secret]`;

  const fetchListeners = async () => {
    const data = await apiRequest('/v1/listeners');
    setListeners(Array.isArray(data) ? data : []);
  };

  const fetchEvents = async (tag = null) => {
    setFetching(true);
    try {
      const path = tag
        ? `/api/events/by-tag?tag=${encodeURIComponent(tag)}&limit=50`
        : '/api/events';
      const data = await apiRequest(path);
      setEvents(Array.isArray(data) ? data : []);
    } finally {
      setFetching(false);
    }
  };

  useEffect(() => {
    if (!isAuthenticated) return;
    fetchListeners().catch((err) => console.error('Failed to fetch listeners', err));
    if (activeTab === 'storyboard') {
      fetchEvents(activeTag).catch((err) => console.error('Failed to fetch events', err));
    }
  }, [isAuthenticated, activeTab, activeTag]);

  const refreshAll = async () => {
    await Promise.allSettled([fetchListeners(), fetchEvents(activeTag)]);
  };

  const handleTagClick = (tag) => {
    setActiveTag(prev => prev === tag ? null : tag);
  };

  const reclassifyEvent = async (eventID) => {
    setReclassifyingEventIDs((current) => ({ ...current, [eventID]: true }));
    try {
      const result = await apiRequest(`/api/events/${eventID}/re-run`, { method: 'POST' });
      const updatedEvent = result?.event;
      if (updatedEvent?.id) {
        setEvents((current) => current.map((event) => (event.id === updatedEvent.id ? updatedEvent : event)));
      } else {
        await fetchEvents(activeTag);
      }
    } catch (err) {
      console.error('Failed to reclassify event', err);
    } finally {
      setReclassifyingEventIDs((current) => {
        const next = { ...current };
        delete next[eventID];
        return next;
      });
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen bg-surface flex items-center justify-center">
        <div className="w-8 h-8 border-2 border-primary border-t-transparent rounded-full animate-spin" />
      </div>
    );
  }

  if (!user) {
    return <MarketingHome login={login} error={error} />;
  }

  return (
    <div className="min-h-screen pb-24 bg-surface text-on-surface">
      <TopAppBar user={user} onLogout={logout} />

      <main className="pt-20 px-4 max-w-md mx-auto">
        <AnimatePresence mode="wait">
          {activeTab === 'storyboard' && (
            <motion.div
              key="storyboard"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
            >
              <Metrics isAuthenticated={isAuthenticated} />

              <section className="mb-8">
                <div className="bg-indigo-500/5 border border-indigo-500/20 rounded-xl p-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-indigo-400 font-label-caps text-[10px]">YOUR INGRESS URL (AUTO)</span>
                    <RefreshCw
                      size={14}
                      className={`text-indigo-400 cursor-pointer ${fetching ? 'animate-spin' : ''}`}
                      onClick={() => refreshAll().catch((err) => console.error(err))}
                    />
                  </div>
                  <div className="flex items-center gap-2 bg-slate-950/50 px-3 py-2 rounded-lg border border-slate-800">
                    <code className="text-indigo-300 font-code-snippet text-xs truncate">{ingressTemplate}</code>
                    <CopyButton
                      value={ingressTemplate}
                      copiedKey={copied}
                      setCopiedKey={setCopied}
                      copyKey="storyboard-ingress"
                    />
                  </div>
                </div>
              </section>

              <section className="space-y-4">
                <div className="flex items-center justify-between px-1">
                  <h2 className="text-on-background">Storyboard</h2>
                  <span className="text-slate-500 text-xs font-medium uppercase tracking-wider">
                    {fetching ? 'Syncing...' : 'Live'}
                  </span>
                </div>

                {!fetching && (listeners.length > 0 || events.length > 0) && (
                  <p className="px-1 text-slate-500 text-xs">
                    Showing {events.length} recent events across {listeners.length} configured hooks.
                  </p>
                )}

                {events.length === 0 && !fetching && (
                  <div className="py-12 text-center space-y-3">
                    <div className="inline-flex p-4 bg-slate-900 rounded-full border border-slate-800 text-slate-500">
                      <RefreshCw size={32} />
                    </div>
                    <p className="text-slate-400 text-sm">
                      No events detected yet.
                      <br />
                      Send a payload to your ingress URL.
                    </p>
                  </div>
                )}

                {activeTag && (
                  <div className="flex items-center gap-2 px-1 mb-2">
                    <span className="text-xs text-indigo-400">Filtered by tag:</span>
                    <span className="text-xs font-semibold text-indigo-300 bg-indigo-500/10 border border-indigo-500/20 px-2 py-0.5 rounded-full">
                      {activeTag}
                    </span>
                    <button
                      onClick={() => setActiveTag(null)}
                      className="text-xs text-slate-500 hover:text-white underline"
                    >
                      Clear
                    </button>
                  </div>
                )}

                {events.map((event, i) => (
                  <StoryboardCard
                    key={event.id || i}
                    event={{
                      id: event.id,
                      status: event.status,
                      time: new Date(event.created_at).toLocaleTimeString([], {
                        hour: '2-digit',
                        minute: '2-digit',
                        second: '2-digit',
                      }),
                      processedText: event.processed_text || '',
                      rawPayload: event.raw_payload_json || event.payload_json || '',
                      tagsJson: event.tags_json || '[]',
                      typeKey: event.type_key || 'webhook',
                      actions: event.action_selected ? [event.action_selected] : ['LOGGED'],
                      reclassifying: !!reclassifyingEventIDs[event.id],
                    }}
                    onTagClick={handleTagClick}
                    onReclassify={reclassifyEvent}
                  />
                ))}
              </section>
            </motion.div>
          )}

          {activeTab === 'urls' && (
            <UrlsTab
              key="urls"
              listeners={listeners}
              user={user}
              setUser={setUser}
              onRefresh={refreshAll}
              copied={copied}
              setCopied={setCopied}
            />
          )}

          {activeTab === 'skills' && (
            <SkillsTab
              key="skills"
              listeners={listeners}
              user={user}
              copied={copied}
              setCopied={setCopied}
              onRefreshListeners={fetchListeners}
            />
          )}

          {activeTab === 'integrations' && (
            <motion.div
              key="integrations"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="space-y-4"
            >
              <IntegrationsTab listeners={listeners} />
            </motion.div>
          )}

          {activeTab === 'byok' && (
            <motion.div
              key="byok"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="space-y-6"
            >
              <h2 className="px-1 text-white">BYOK</h2>
              <BYOKSettings />
            </motion.div>
          )}
        </AnimatePresence>
      </main>

      <button
        onClick={() => setActiveTab(activeTab === 'urls' ? 'skills' : 'urls')}
        className="fixed right-6 bottom-24 w-14 h-14 bg-primary-container text-on-primary-container rounded-full shadow-2xl flex items-center justify-center active:scale-90 transition-transform duration-150 z-40 border border-white/10"
        title={activeTab === 'urls' ? 'Open skills tools' : 'Open URL tools'}
      >
        <Plus size={32} />
      </button>

      <BottomNavBar activeTab={activeTab} onTabChange={setActiveTab} />
    </div>
  );
}

const BYOKSettings = () => {
  const [provider, setProvider] = useState('groq');
  const [apiKey, setApiKey] = useState('');
  const [baseURL, setBaseURL] = useState('');
  const [model, setModel] = useState('');
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState('');
  const [providers, setProviders] = useState([]);
  const [loadingProviders, setLoadingProviders] = useState(false);

  const fetchProviders = async () => {
    setLoadingProviders(true);
    try {
      const data = await apiRequest('/v1/byok/providers');
      setProviders(Array.isArray(data) ? data : []);
    } catch (err) {
      setNotice(err.message);
    } finally {
      setLoadingProviders(false);
    }
  };

  useEffect(() => {
    fetchProviders();
  }, []);

  const save = async () => {
    setSaving(true);
    setNotice('');
    try {
      await apiRequest('/v1/byok/providers', {
        method: 'POST',
        body: JSON.stringify({ provider, api_key: apiKey, base_url: baseURL, model, is_default: true }),
      });
      setApiKey('');
      setBaseURL('');
      setModel('');
      await fetchProviders();
      setNotice('Provider config saved.');
    } catch (err) {
      setNotice(err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Panel
      title="LLM Providers"
      subtitle="Manage your provider credentials separately from webhook settings, with model and endpoint overrides when needed."
    >
      <div className="space-y-3">
        <FormField label="Provider">
          <Select value={provider} onChange={(e) => setProvider(e.target.value)}>
            <option value="groq">Groq (Recommended)</option>
            <option value="cerebras">Cerebras</option>
            <option value="openai">OpenAI</option>
            <option value="openrouter">OpenRouter</option>
          </Select>
        </FormField>
        <FormField label="API Key">
          <TextInput
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder="sk-..."
          />
        </FormField>
        <div className="grid grid-cols-2 gap-3">
          <FormField label="Base URL">
            <TextInput
              value={baseURL}
              onChange={(e) => setBaseURL(e.target.value)}
              placeholder="Optional override"
            />
          </FormField>
          <FormField label="Model">
            <TextInput
              value={model}
              onChange={(e) => setModel(e.target.value)}
              placeholder="Optional override"
            />
          </FormField>
        </div>
        {notice && <InlineNotice tone={notice.includes('saved') ? 'success' : 'error'}>{notice}</InlineNotice>}
        <button
          onClick={save}
          disabled={saving}
          className="w-full bg-primary text-on-primary font-bold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
        >
          {saving ? 'SAVING...' : 'SAVE CONFIG'}
        </button>

        <div className="space-y-2 pt-2">
          <div className="flex items-center justify-between px-1">
            <p className="text-[10px] text-slate-500 font-label-caps">Saved Providers</p>
            {loadingProviders && <RefreshCw size={12} className="text-slate-500 animate-spin" />}
          </div>
          {providers.map((item) => (
            <div key={item.id || item.provider} className="rounded-xl border border-slate-800 bg-slate-950/40 p-3 space-y-1">
              <div className="flex items-center justify-between gap-2">
                <span className="text-sm text-white font-medium">{item.provider}</span>
                <StatusBadge status={item.is_default ? 'ACTIVE' : 'SHADOW'} />
              </div>
              <p className="text-[11px] text-slate-400 break-all">{item.model || 'default model'}</p>
              <p className="text-[11px] text-slate-500 break-all">{item.base_url || 'default endpoint'}</p>
            </div>
          ))}
          {!providers.length && !loadingProviders && (
            <p className="text-slate-500 text-xs text-center py-3">No BYOK providers saved yet.</p>
          )}
        </div>
      </div>
    </Panel>
  );
};

const IntegrationsTab = ({ listeners }) => {
  const defaultTargetForm = () => ({
    target_key: '',
    target_type: 'http',
    purpose: '',
    enabled: true,
    allowed_actions: ['forward_http'],
    auth_type: 'none',
    auth_secret_ref: '',
    auth_header_name: '',
    auth_prefix: '',
    auth_query_param: '',
    auth_env_var: '',
    config_text: prettyJSON({ url: 'https://example.com/webhook', method: 'POST', headers: { 'x-agenthook-source': 'listener' } }),
    schema_text: prettyJSON({}),
    header_secret_refs_text: prettyJSON({}),
    header_env_refs_text: prettyJSON({}),
  });
  const defaultSecretForm = () => ({
    secret_key: '',
    purpose: '',
    secret_value: '',
  });

  const [targets, setTargets] = useState([]);
  const [secrets, setSecrets] = useState([]);
  const [loadingTargets, setLoadingTargets] = useState(false);
  const [loadingSecrets, setLoadingSecrets] = useState(false);
  const [savingTarget, setSavingTarget] = useState(false);
  const [savingSecret, setSavingSecret] = useState(false);
  const [notice, setNotice] = useState('');
  const [expandedTargetID, setExpandedTargetID] = useState('');
  const [editingTargetID, setEditingTargetID] = useState('');
  const [editingSecretID, setEditingSecretID] = useState('');
  const [targetForm, setTargetForm] = useState(defaultTargetForm);
  const [secretForm, setSecretForm] = useState(defaultSecretForm);

  const hasSingleTenant = listeners.some((listener) => listener.deployment_mode === 'single_tenant');

  const fetchTargets = async () => {
    setLoadingTargets(true);
    try {
      const data = await apiRequest('/api/forward-targets');
      setTargets(Array.isArray(data) ? data : []);
    } catch (err) {
      setNotice(err.message);
    } finally {
      setLoadingTargets(false);
    }
  };

  const fetchSecrets = async () => {
    setLoadingSecrets(true);
    try {
      const data = await apiRequest('/api/integration-secrets');
      setSecrets(Array.isArray(data) ? data : []);
    } catch (err) {
      setNotice(err.message);
    } finally {
      setLoadingSecrets(false);
    }
  };

  useEffect(() => {
    fetchTargets();
    fetchSecrets();
  }, []);

  const applyIntegrationPreset = (presetKey) => {
    const preset = INTEGRATION_PRESETS[presetKey];
    if (!preset) return;
    const auth = preset.auth || {};
    setTargetForm({
      target_key: preset.target_key,
      target_type: preset.target_type,
      purpose: preset.purpose,
      enabled: preset.enabled,
      allowed_actions: preset.allowed_actions,
      auth_type: auth.type || 'none',
      auth_secret_ref: auth.secret_ref || '',
      auth_header_name: auth.header_name || '',
      auth_prefix: auth.prefix || '',
      auth_query_param: auth.query_param || '',
      auth_env_var: auth.env_var || '',
      config_text: prettyJSON(preset.config),
      schema_text: prettyJSON(preset.schema),
      header_secret_refs_text: prettyJSON({}),
      header_env_refs_text: prettyJSON({}),
    });
    setEditingTargetID('');
    setNotice(`${preset.target_key} template loaded. Attach a named secret ref or rely on single-tenant env fallback.`);
  };

  const persistTarget = async () => {
    setSavingTarget(true);
    setNotice('');
    try {
      const payload = {
        target_key: targetForm.target_key,
        target_type: targetForm.target_type === 'openclaw' ? 'http' : targetForm.target_type,
        purpose: targetForm.purpose,
        enabled: targetForm.enabled,
        allowed_actions: targetForm.allowed_actions,
        config: parseObjectOrThrow(targetForm.config_text, 'Config JSON'),
        schema: parseObjectOrThrow(targetForm.schema_text, 'Schema JSON'),
        auth: {
          type: targetForm.auth_type === 'none' ? '' : targetForm.auth_type,
          secret_ref: targetForm.auth_secret_ref,
          header_name: targetForm.auth_header_name,
          prefix: targetForm.auth_prefix,
          query_param: targetForm.auth_query_param,
          env_var: targetForm.auth_env_var,
        },
        header_secret_refs: parseObjectOrThrow(targetForm.header_secret_refs_text, 'Header secret refs JSON'),
        header_env_refs: parseObjectOrThrow(targetForm.header_env_refs_text, 'Header env refs JSON'),
      };
      if (!payload.target_key.trim()) {
        throw new Error('target_key is required');
      }
      const path = editingTargetID ? `/api/forward-targets/${editingTargetID}` : '/api/forward-targets';
      const method = editingTargetID ? 'PUT' : 'POST';
      await apiRequest(path, {
        method,
        body: JSON.stringify(payload),
      });
      await fetchTargets();
      setEditingTargetID('');
      setTargetForm(defaultTargetForm());
      setNotice(`Integration ${method === 'POST' ? 'created' : 'updated'} successfully.`);
    } catch (err) {
      setNotice(err.message);
    } finally {
      setSavingTarget(false);
    }
  };

  const persistSecret = async () => {
    setSavingSecret(true);
    setNotice('');
    try {
      if (!secretForm.secret_key.trim()) {
        throw new Error('secret_key is required');
      }
      if (!editingSecretID && !secretForm.secret_value.trim()) {
        throw new Error('secret_value is required');
      }
      const path = editingSecretID ? `/api/integration-secrets/${editingSecretID}` : '/api/integration-secrets';
      const method = editingSecretID ? 'PUT' : 'POST';
      await apiRequest(path, {
        method,
        body: JSON.stringify(secretForm),
      });
      await fetchSecrets();
      setEditingSecretID('');
      setSecretForm(defaultSecretForm());
      setNotice(`Integration secret ${method === 'POST' ? 'created' : 'updated'} successfully.`);
    } catch (err) {
      setNotice(err.message);
    } finally {
      setSavingSecret(false);
    }
  };

  const beginEditTarget = (target) => {
    const details = targetRecordDetails(target);
    const config = targetConfigFromRecord(target);
    const schema = objectFromJSONText(target.schema_json);
    const allowedActions = arrayFromJSONText(target.allowed_actions_json);
    const auth = details.auth || {};
    setEditingTargetID(target.id);
    setExpandedTargetID(target.id);
    setTargetForm({
      target_key: target.target_key || '',
      target_type: target.target_type || 'http',
      purpose: target.purpose || '',
      enabled: target.enabled !== false,
      allowed_actions: allowedActions.length ? allowedActions : ['forward_http'],
      auth_type: auth.type || 'none',
      auth_secret_ref: auth.secret_ref || '',
      auth_header_name: auth.header_name || '',
      auth_prefix: auth.prefix || '',
      auth_query_param: auth.query_param || '',
      auth_env_var: auth.env_var || '',
      config_text: prettyJSON(config),
      schema_text: prettyJSON(schema),
      header_secret_refs_text: prettyJSON(details.header_secret_refs || {}),
      header_env_refs_text: prettyJSON(details.header_env_refs || {}),
    });
  };

  const beginEditSecret = (secret) => {
    setEditingSecretID(secret.id);
    setSecretForm({
      secret_key: secret.secret_key || '',
      purpose: secret.purpose || '',
      secret_value: '',
    });
  };

  const deleteTarget = async (target) => {
    if (!window.confirm(`Delete integration "${target.target_key || target.id}"?`)) return;
    try {
      await apiRequest(`/api/forward-targets/${target.id}`, { method: 'DELETE' });
      if (editingTargetID === target.id) {
        setEditingTargetID('');
        setTargetForm(defaultTargetForm());
      }
      await fetchTargets();
      setNotice('Integration deleted.');
    } catch (err) {
      setNotice(err.message);
    }
  };

  const deleteSecret = async (secret) => {
    if (!window.confirm(`Delete integration secret "${secret.secret_key}"?`)) return;
    try {
      await apiRequest(`/api/integration-secrets/${secret.id}`, { method: 'DELETE' });
      if (editingSecretID === secret.id) {
        setEditingSecretID('');
        setSecretForm(defaultSecretForm());
      }
      await fetchSecrets();
      setNotice('Integration secret deleted.');
    } catch (err) {
      setNotice(err.message);
    }
  };

  const toggleAction = (action) => {
    setTargetForm((current) => {
      const exists = current.allowed_actions.includes(action);
      return {
        ...current,
        allowed_actions: exists
          ? current.allowed_actions.filter((item) => item !== action)
          : [...current.allowed_actions, action],
      };
    });
  };

  return (
    <motion.div
      initial={{ opacity: 0, x: -20 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: 20 }}
      className="space-y-4"
    >
      <h2 className="px-1 text-white">Integrations</h2>

      <Panel
        title="Integration Secrets"
        subtitle="Store named secret refs for multitenant accounts. Single-tenant listeners can also fall back to env vars automatically."
        action={<KeyRound size={18} className="text-primary" />}
      >
        <InlineNotice>
          {hasSingleTenant
            ? 'Single-tenant listeners can auto-resolve conventional env vars when no secret ref is attached. Multitenant listeners should use named secret refs.'
            : 'Multitenant listeners should attach named secret refs. Env fallback is kept as an operator override only.'}
        </InlineNotice>
        <div className="grid grid-cols-1 gap-3">
          <FormField label="Secret Key" hint="Stable reference like openclaw_api_key or crm_bearer_token.">
            <TextInput
              value={secretForm.secret_key}
              onChange={(e) => setSecretForm((current) => ({ ...current, secret_key: e.target.value }))}
              placeholder="openclaw_api_key"
            />
          </FormField>
          <FormField label="Purpose">
            <TextInput
              value={secretForm.purpose}
              onChange={(e) => setSecretForm((current) => ({ ...current, purpose: e.target.value }))}
              placeholder="Bearer token for OpenClaw intake API"
            />
          </FormField>
          <FormField label={editingSecretID ? 'Rotate Secret Value (optional)' : 'Secret Value'}>
            <TextInput
              type="password"
              value={secretForm.secret_value}
              onChange={(e) => setSecretForm((current) => ({ ...current, secret_value: e.target.value }))}
              placeholder={editingSecretID ? 'Leave blank to keep current value' : 'Paste token'}
            />
          </FormField>
        </div>
        <div className="flex gap-2">
          <button
            onClick={persistSecret}
            disabled={savingSecret}
            className="flex-1 bg-primary text-on-primary font-bold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
          >
            {savingSecret ? 'SAVING...' : editingSecretID ? 'SAVE SECRET' : 'CREATE SECRET'}
          </button>
          {editingSecretID && (
            <button
              onClick={() => {
                setEditingSecretID('');
                setSecretForm(defaultSecretForm());
              }}
              className="px-4 border border-slate-800 rounded-lg text-sm text-slate-200 hover:bg-slate-900"
            >
              Cancel
            </button>
          )}
        </div>
        <div className="space-y-2 pt-2">
          <div className="flex items-center justify-between px-1">
            <p className="text-[10px] text-slate-500 font-label-caps">Saved Secret Refs</p>
            {loadingSecrets && <RefreshCw size={12} className="text-slate-500 animate-spin" />}
          </div>
          {secrets.map((secret) => (
            <div key={secret.id} className="rounded-xl border border-slate-800 bg-slate-950/40 p-3 space-y-2">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <p className="text-sm text-white font-medium">{secret.secret_key}</p>
                  <p className="text-[11px] text-slate-500">{secret.purpose || 'No purpose provided'}</p>
                </div>
                <div className="flex gap-2">
                  <button onClick={() => beginEditSecret(secret)} className="px-2 py-1 rounded-lg border border-slate-700 text-[11px] text-slate-200 hover:bg-slate-900">
                    Rotate
                  </button>
                  <button onClick={() => deleteSecret(secret)} className="px-2 py-1 rounded-lg border border-red-900/60 text-[11px] text-red-300 hover:bg-red-950/40">
                    Delete
                  </button>
                </div>
              </div>
            </div>
          ))}
          {!secrets.length && !loadingSecrets && (
            <p className="text-slate-500 text-xs text-center py-3">No integration secrets saved yet.</p>
          )}
        </div>
      </Panel>

      <Panel
        title="Create Integration"
        subtitle="Define reusable named targets for OpenClaw, custom forward URLs, or any downstream system your skills can call."
        action={<Cable size={18} className="text-primary" />}
      >
        <div className="flex flex-wrap gap-2">
          <button
            onClick={() => applyIntegrationPreset('openclaw')}
            className="px-3 py-1.5 rounded-lg border border-slate-700 text-xs text-slate-200 hover:bg-slate-900"
          >
            Load OpenClaw Template
          </button>
          <button
            onClick={() => applyIntegrationPreset('forward_url')}
            className="px-3 py-1.5 rounded-lg border border-slate-700 text-xs text-slate-200 hover:bg-slate-900"
          >
            Load Forward URL Template
          </button>
        </div>
        <div className="grid grid-cols-1 gap-3">
          <FormField label="Target Key" hint="Skills and router outputs reference this stable key.">
            <TextInput
              value={targetForm.target_key}
              onChange={(e) => setTargetForm((current) => ({ ...current, target_key: e.target.value }))}
              placeholder="openclaw_primary"
            />
          </FormField>
          <div className="grid grid-cols-2 gap-3">
            <FormField label="Target Type">
              <Select
                value={targetForm.target_type}
                onChange={(e) => setTargetForm((current) => ({ ...current, target_type: e.target.value }))}
              >
                {INTEGRATION_TARGET_TYPES.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </Select>
            </FormField>
            <FormField label="Enabled">
              <Select
                value={String(targetForm.enabled)}
                onChange={(e) => setTargetForm((current) => ({ ...current, enabled: e.target.value === 'true' }))}
              >
                <option value="true">Enabled</option>
                <option value="false">Disabled</option>
              </Select>
            </FormField>
          </div>
          <FormField label="Purpose">
            <TextInput
              value={targetForm.purpose}
              onChange={(e) => setTargetForm((current) => ({ ...current, purpose: e.target.value }))}
              placeholder="Forward leads to OpenClaw or a generic intake URL."
            />
          </FormField>
          <FormField label="Allowed Actions">
            <div className="flex flex-wrap gap-2">
              {INTEGRATION_ACTION_OPTIONS.map((action) => (
                <label key={action} className="inline-flex items-center gap-2 rounded-lg border border-slate-800 bg-slate-950/40 px-3 py-2 text-xs text-slate-200">
                  <input
                    type="checkbox"
                    checked={targetForm.allowed_actions.includes(action)}
                    onChange={() => toggleAction(action)}
                  />
                  {action}
                </label>
              ))}
            </div>
          </FormField>
          <div className="grid grid-cols-2 gap-3">
            <FormField label="Auth Type">
              <Select
                value={targetForm.auth_type}
                onChange={(e) => setTargetForm((current) => ({ ...current, auth_type: e.target.value }))}
              >
                {INTEGRATION_AUTH_TYPES.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </Select>
            </FormField>
            <FormField label="Secret Ref">
              <Select
                value={targetForm.auth_secret_ref}
                onChange={(e) => setTargetForm((current) => ({ ...current, auth_secret_ref: e.target.value }))}
              >
                <option value="">None</option>
                {secrets.map((secret) => (
                  <option key={secret.id} value={secret.secret_key}>
                    {secret.secret_key}
                  </option>
                ))}
              </Select>
            </FormField>
          </div>
          {targetForm.auth_type !== 'none' && (
            <div className="grid grid-cols-2 gap-3">
              <FormField label="Header Name">
                <TextInput
                  value={targetForm.auth_header_name}
                  onChange={(e) => setTargetForm((current) => ({ ...current, auth_header_name: e.target.value }))}
                  placeholder={targetForm.auth_type === 'query_param' ? 'Unused for query param auth' : 'Authorization'}
                />
              </FormField>
              <FormField label={targetForm.auth_type === 'query_param' ? 'Query Param' : 'Prefix'}>
                <TextInput
                  value={targetForm.auth_type === 'query_param' ? targetForm.auth_query_param : targetForm.auth_prefix}
                  onChange={(e) =>
                    setTargetForm((current) => ({
                      ...current,
                      ...(targetForm.auth_type === 'query_param'
                        ? { auth_query_param: e.target.value }
                        : { auth_prefix: e.target.value }),
                    }))
                  }
                  placeholder={targetForm.auth_type === 'query_param' ? 'api_key' : 'Bearer '}
                />
              </FormField>
            </div>
          )}
          <FormField label="Explicit Env Var Override" hint="Optional env var to use after single-tenant conventional fallback.">
            <TextInput
              value={targetForm.auth_env_var}
              onChange={(e) => setTargetForm((current) => ({ ...current, auth_env_var: e.target.value }))}
              placeholder="OPENCLAW_API_KEY"
            />
          </FormField>
          <FormField label="Config JSON" hint="Store the endpoint, static headers, and destination-specific options here.">
            <TextArea
              value={targetForm.config_text}
              onChange={(e) => setTargetForm((current) => ({ ...current, config_text: e.target.value }))}
              className="min-h-36 font-code-snippet"
            />
          </FormField>
          <FormField label="Header Secret Refs JSON" hint='Optional per-header secret refs, for example `{ "X-API-Key": "crm_bearer_token" }`. '>
            <TextArea
              value={targetForm.header_secret_refs_text}
              onChange={(e) => setTargetForm((current) => ({ ...current, header_secret_refs_text: e.target.value }))}
              className="min-h-24 font-code-snippet"
            />
          </FormField>
          <FormField label="Header Env Refs JSON" hint='Optional per-header env refs, for example `{ "X-Webhook-Token": "OPENCLAW_TOKEN" }`. '>
            <TextArea
              value={targetForm.header_env_refs_text}
              onChange={(e) => setTargetForm((current) => ({ ...current, header_env_refs_text: e.target.value }))}
              className="min-h-24 font-code-snippet"
            />
          </FormField>
          <FormField label="Schema JSON" hint="Optional hints for what params the skill or router should produce.">
            <TextArea
              value={targetForm.schema_text}
              onChange={(e) => setTargetForm((current) => ({ ...current, schema_text: e.target.value }))}
              className="min-h-24 font-code-snippet"
            />
          </FormField>
        </div>
        {notice && <InlineNotice tone={notice.toLowerCase().includes('success') || notice.toLowerCase().includes('created') || notice.toLowerCase().includes('updated') ? 'success' : 'info'}>{notice}</InlineNotice>}
        <div className="flex gap-2">
          <button
            onClick={persistTarget}
            disabled={savingTarget}
            className="flex-1 bg-primary text-on-primary font-bold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
          >
            {savingTarget ? 'SAVING...' : editingTargetID ? 'SAVE INTEGRATION' : 'CREATE INTEGRATION'}
          </button>
          {editingTargetID && (
            <button
              onClick={() => {
                setEditingTargetID('');
                setTargetForm(defaultTargetForm());
              }}
              className="px-4 border border-slate-800 rounded-lg text-sm text-slate-200 hover:bg-slate-900"
            >
              Reset
            </button>
          )}
        </div>
      </Panel>

      <Panel
        title="Configured Integrations"
        subtitle="Every saved target is reusable across skills and router outputs. Expand a card to inspect auth wiring, schema, and destination config."
        action={
          <button onClick={fetchTargets} className="text-slate-400 hover:text-white" title="Refresh integrations">
            <RefreshCw size={16} className={loadingTargets ? 'animate-spin' : ''} />
          </button>
        }
      >
        <div className="space-y-3">
          {targets.map((target) => {
            const details = targetRecordDetails(target);
            const allowedActions = arrayFromJSONText(target.allowed_actions_json);
            const config = targetConfigFromRecord(target);
            const schema = objectFromJSONText(target.schema_json);
            const auth = details.auth || {};
            const headerSecretRefs = details.header_secret_refs || {};
            const headerEnvRefs = details.header_env_refs || {};
            const isExpanded = expandedTargetID === target.id;
            return (
              <div
                key={target.id}
                className="rounded-2xl border border-slate-800 bg-slate-950/30 p-4 space-y-3 cursor-pointer"
                onClick={() => setExpandedTargetID((current) => (current === target.id ? '' : target.id))}
                role="button"
                tabIndex={0}
                onKeyDown={(event) => {
                  if (event.target !== event.currentTarget) return;
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    setExpandedTargetID((current) => (current === target.id ? '' : target.id));
                  }
                }}
              >
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-white text-sm font-medium">{target.target_key || target.id}</p>
                    <p className="text-slate-500 text-[11px] mt-1">{target.target_type} · {target.purpose || 'No purpose yet'}</p>
                  </div>
                  <div className="flex items-center gap-2">
                    <StatusBadge status={target.enabled ? 'ACTIVE' : 'LEARNING'} />
                    {isExpanded ? <ChevronUp size={16} className="text-slate-500" /> : <ChevronDown size={16} className="text-slate-500" />}
                  </div>
                </div>
                {isExpanded && (
                  <div className="space-y-3 border-t border-slate-800/80 pt-3" onClick={(event) => event.stopPropagation()}>
                    <div className="flex flex-wrap gap-2">
                      {allowedActions.map((action) => (
                        <span key={action} className="rounded-full border border-slate-700 px-2 py-1 text-[10px] text-slate-300">
                          {action}
                        </span>
                      ))}
                      {!allowedActions.length && (
                        <span className="rounded-full border border-slate-700 px-2 py-1 text-[10px] text-slate-500">
                          default actions
                        </span>
                      )}
                    </div>
                    <div className="grid grid-cols-1 gap-2 text-[11px] text-slate-400">
                      <div>Created: <span className="text-slate-300">{target.created_at ? new Date(target.created_at).toLocaleString() : 'Unknown'}</span></div>
                      <div>Target ID: <code className="text-slate-300 break-all">{target.id}</code></div>
                      <div>Auth: <span className="text-slate-300">{auth.type || 'none'}</span></div>
                      <div>Secret Ref: <span className="text-slate-300">{auth.secret_ref || 'none'}</span></div>
                      <div>Env Override: <span className="text-slate-300">{auth.env_var || 'none'}</span></div>
                    </div>
                    <FormField label="Config Snapshot">
                      <pre className="text-xs text-slate-300 bg-slate-950/80 p-3 rounded-xl overflow-auto border border-slate-800">{prettyJSON(config)}</pre>
                    </FormField>
                    <FormField label="Header Secret Refs">
                      <pre className="text-xs text-slate-300 bg-slate-950/80 p-3 rounded-xl overflow-auto border border-slate-800">{prettyJSON(headerSecretRefs)}</pre>
                    </FormField>
                    <FormField label="Header Env Refs">
                      <pre className="text-xs text-slate-300 bg-slate-950/80 p-3 rounded-xl overflow-auto border border-slate-800">{prettyJSON(headerEnvRefs)}</pre>
                    </FormField>
                    <FormField label="Schema Snapshot">
                      <pre className="text-xs text-slate-300 bg-slate-950/80 p-3 rounded-xl overflow-auto border border-slate-800">{prettyJSON(schema)}</pre>
                    </FormField>
                    <div className="flex gap-2">
                      <button
                        type="button"
                        onClick={() => beginEditTarget(target)}
                        className="px-3 py-1.5 rounded-lg border border-slate-700 text-xs text-slate-200 hover:bg-slate-900"
                      >
                        <Save size={12} className="inline mr-1" />
                        Edit Integration
                      </button>
                      <button
                        type="button"
                        onClick={() => deleteTarget(target)}
                        className="px-3 py-1.5 rounded-lg border border-red-900/60 text-xs text-red-300 hover:bg-red-950/40"
                      >
                        <Trash2 size={12} className="inline mr-1" />
                        Delete
                      </button>
                    </div>
                  </div>
                )}
              </div>
            );
          })}
          {!targets.length && !loadingTargets && (
            <p className="text-slate-500 text-center py-6">
              No integrations saved yet. Create an OpenClaw or generic forward URL target above.
            </p>
          )}
        </div>
      </Panel>
    </motion.div>
  );
};

const UrlsTab = ({ listeners, user, setUser, onRefresh, copied, setCopied }) => {
  const [provider, setProvider] = useState('github');
  const [listenerID, setListenerID] = useState('');
  const [deploymentMode, setDeploymentMode] = useState('multitenant');
  const [plainTextAction, setPlainTextAction] = useState('store_mysql');
  const [useLLMFallback, setUseLLMFallback] = useState(true);
  const [listenerSecretMode, setListenerSecretMode] = useState('auto');
  const [listenerSecretValue, setListenerSecretValue] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [apiToken, setApiToken] = useState('');
  const [tokenBusy, setTokenBusy] = useState(false);
  const [apiTokensList, setApiTokensList] = useState([]);
  const [loadingTokens, setLoadingTokens] = useState(false);
  const [secretMap, setSecretMap] = useState({});
  const [secretsHistory, setSecretsHistory] = useState({});
  const [loadingSecrets, setLoadingSecrets] = useState(false);
  const [secretComposer, setSecretComposer] = useState({});
  const [publicAliasDraft, setPublicAliasDraft] = useState(user?.public_alias || user?.slug || '');
  const [aliasSaving, setAliasSaving] = useState(false);
  const [aliasNotice, setAliasNotice] = useState('');

  const accountSlug = user?.slug || '[account]';
  const publicAlias = user?.public_alias || user?.slug || '[userkey]';

  useEffect(() => {
    setPublicAliasDraft(user?.public_alias || user?.slug || '');
  }, [user?.public_alias, user?.slug]);

  const fetchTokens = async () => {
    setLoadingTokens(true);
    try {
      const data = await apiRequest('/v1/auth/tokens');
      setApiTokensList(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error('Failed to fetch tokens', err);
    } finally {
      setLoadingTokens(false);
    }
  };

  const fetchSecrets = async (listener) => {
    try {
      const data = await apiRequest(`/v1/listeners/${listener.listener_id}/secrets?provider=${listener.provider}`);
      setSecretsHistory((current) => ({
        ...current,
        [`${listener.provider}:${listener.listener_id}`]: data || [],
      }));
    } catch (err) {
      console.error('Failed to fetch secrets', err);
    }
  };

  useEffect(() => {
    fetchTokens();
    if (listeners.length === 0) return;
    setLoadingSecrets(true);
    Promise.allSettled(listeners.map((listener) => fetchSecrets(listener))).finally(() => setLoadingSecrets(false));
  }, [listeners]);

  const deleteListener = async (listener) => {
    if (!window.confirm(`Delete listener "${listener.listener_id}" (${listener.provider})? This will revoke all its secrets.`)) return;
    try {
      await apiRequest(`/v1/listeners/${listener.listener_id}?provider=${listener.provider}`, { method: 'DELETE' });
      setSecretsHistory((prev) => {
        const next = { ...prev };
        delete next[`${listener.provider}:${listener.listener_id}`];
        return next;
      });
      await onRefresh();
    } catch (err) {
      setError(`Failed to delete listener: ${err.message}`);
    }
  };

  const createListener = async () => {
    setSubmitting(true);
    setError('');
    try {
      const created = await apiRequest('/v1/listeners', {
        method: 'POST',
        body: JSON.stringify({
          provider,
          listener_id: listenerID,
          deployment_mode: deploymentMode,
          plain_text_action: plainTextAction,
          use_llm_fallback: useLLMFallback,
          secret_value: listenerSecretMode === 'manual' ? listenerSecretValue.trim() : '',
        }),
      });
      setSecretMap((current) => ({
        ...current,
        [`${created.provider}:${created.listener_id}`]: created,
      }));
      setListenerID('');
      setListenerSecretValue('');
      setListenerSecretMode('auto');
      await onRefresh();
    } catch (err) {
      setError(err.message);
    } finally {
      setSubmitting(false);
    }
  };

  const updatePublicAlias = async () => {
    setAliasSaving(true);
    setAliasNotice('');
    setError('');
    try {
      const updated = await apiRequest('/api/me', {
        method: 'PUT',
        body: JSON.stringify({ public_alias: publicAliasDraft.trim() }),
      });
      setUser(updated);
      setAliasNotice('Public webhook alias saved.');
      await onRefresh();
    } catch (err) {
      setError(err.message);
    } finally {
      setAliasSaving(false);
    }
  };

  const createSecret = async (listener) => {
    const key = `${listener.provider}:${listener.listener_id}`;
    const draft = secretComposer[key] || { mode: 'auto', secret_value: '' };
    try {
      const created = await apiRequest(`/v1/listeners/${listener.listener_id}/secrets`, {
        method: 'POST',
        body: JSON.stringify({
          provider: listener.provider,
          secret_value: draft.mode === 'manual' ? draft.secret_value.trim() : '',
        }),
      });
      setSecretMap((current) => ({ ...current, [key]: created }));
      setSecretComposer((current) => ({
        ...current,
        [key]: { mode: 'auto', secret_value: '' },
      }));
      await fetchSecrets(listener);
    } catch (err) {
      setError(err.message);
    }
  };

  const createToken = async () => {
    setTokenBusy(true);
    setError('');
    try {
      const created = await apiRequest('/v1/auth/tokens', {
        method: 'POST',
      });
      setApiToken(created?.token || '');
      await fetchTokens();
    } catch (err) {
      setError(err.message);
    } finally {
      setTokenBusy(false);
    }
  };

  const revokeToken = async (id) => {
    if (!window.confirm('Revoke this token? Any scripts using it will fail immediately.')) return;
    try {
      await apiRequest(`/v1/auth/tokens/${id}`, { method: 'DELETE' });
      await fetchTokens();
    } catch (err) {
      setError(err.message);
    }
  };

  return (
    <motion.div
      initial={{ opacity: 0, x: -20 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: 20 }}
      className="space-y-4"
    >
      <h2 className="px-1 text-white">Webhook URLs</h2>

      <Panel
        title="Public Webhook Alias"
        subtitle="This alias becomes both the short webhook URL and the inbox identity for each active secret."
        action={<BadgeCheck size={18} className="text-primary" />}
      >
        <div className="space-y-3">
          <FormField label="Userkey" hint="Seeded from your current slug, globally unique, and editable later.">
            <TextInput
              value={publicAliasDraft}
              onChange={(e) => setPublicAliasDraft(e.target.value)}
              placeholder="abhinaviitg18"
            />
          </FormField>
          <div className="rounded-xl border border-slate-800 bg-slate-950/50 px-3 py-2 text-xs text-slate-300">
            <div>Canonical URL: <code className="text-indigo-300 break-all">https://app.agenthook.store/{publicAlias}.[secret]</code></div>
            <div className="mt-1">Inbox address: <code className="text-slate-200 break-all">{publicAlias}.[secret]@app.agenthook.store</code></div>
            <div className="mt-1 text-slate-500">Changing your userkey changes the canonical inbox address for future secrets too.</div>
          </div>
          {aliasNotice && <InlineNotice tone="success">{aliasNotice}</InlineNotice>}
          {error && <InlineNotice tone="error">{error}</InlineNotice>}
          <button
            onClick={updatePublicAlias}
            disabled={aliasSaving}
            className="w-full bg-slate-900 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
          >
            {aliasSaving ? 'SAVING...' : 'SAVE USERKEY'}
          </button>
        </div>
      </Panel>

      <Panel
        title="Create Listener"
        subtitle="Provision a new ingress scenario directly from the UI, then bind it to a generated or custom secret."
        action={<Link2 size={18} className="text-primary" />}
      >
        <div className="grid grid-cols-1 gap-3">
          <FormField label="Provider" hint="Choose a provider label or reuse one of the existing API-friendly options.">
            <Select value={provider} onChange={(e) => setProvider(e.target.value)}>
              {PROVIDER_OPTIONS.map((item) => (
                <option key={item} value={item}>
                  {item}
                </option>
              ))}
            </Select>
          </FormField>
          <FormField label="Listener ID" hint="Optional. Leave blank to let the backend generate one.">
            <TextInput
              value={listenerID}
              onChange={(e) => setListenerID(e.target.value)}
              placeholder="jobs-feed"
            />
          </FormField>
          <div className="grid grid-cols-2 gap-3">
            <FormField label="Deployment Mode">
              <Select value={deploymentMode} onChange={(e) => setDeploymentMode(e.target.value)}>
                <option value="multitenant">Multitenant</option>
                <option value="single_tenant">Single Tenant</option>
              </Select>
            </FormField>
            <FormField label="Default Action">
              <Select value={plainTextAction} onChange={(e) => setPlainTextAction(e.target.value)}>
                {FORCED_ACTION_OPTIONS.map((item) => (
                  <option key={item} value={item}>
                    {item}
                  </option>
                ))}
              </Select>
            </FormField>
          </div>
          <label className="flex items-center gap-2 text-sm text-slate-300">
            <input
              type="checkbox"
              checked={useLLMFallback}
              onChange={(e) => setUseLLMFallback(e.target.checked)}
            />
            Use LLM fallback when deterministic logic is insufficient
          </label>
          <div className="space-y-2">
            <p className="text-[10px] text-slate-500 font-label-caps">Initial Secret</p>
            <div className="grid grid-cols-2 gap-2">
              <button
                type="button"
                onClick={() => setListenerSecretMode('auto')}
                className={`rounded-lg border px-3 py-2 text-xs font-semibold ${listenerSecretMode === 'auto' ? 'border-primary bg-primary/10 text-white' : 'border-slate-800 text-slate-400'}`}
              >
                Generate automatically
              </button>
              <button
                type="button"
                onClick={() => setListenerSecretMode('manual')}
                className={`rounded-lg border px-3 py-2 text-xs font-semibold ${listenerSecretMode === 'manual' ? 'border-primary bg-primary/10 text-white' : 'border-slate-800 text-slate-400'}`}
              >
                Set my own secret
              </button>
            </div>
            {listenerSecretMode === 'manual' && (
              <TextInput
                value={listenerSecretValue}
                onChange={(e) => setListenerSecretValue(e.target.value)}
                placeholder="leadrouter_2026"
              />
            )}
          </div>
          {error && <InlineNotice tone="error">{error}</InlineNotice>}
          <button
            onClick={createListener}
            disabled={submitting}
            className="w-full bg-primary text-on-primary font-bold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
          >
            {submitting ? 'CREATING...' : 'CREATE LISTENER'}
          </button>
        </div>
      </Panel>

      <Panel
        title="API Tokens"
        subtitle="Manage and generate tokens for curl, scripts, or direct API testing."
        action={<KeyRound size={18} className="text-primary" />}
      >
        <button
          onClick={createToken}
          disabled={tokenBusy}
          className="w-full bg-slate-900 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
        >
          {tokenBusy ? 'CREATING...' : 'CREATE API TOKEN'}
        </button>
        {apiToken && (
          <div className="flex flex-col gap-2 bg-slate-950/50 px-3 py-2 rounded-lg border border-slate-800">
            <span className="text-[10px] text-emerald-400 font-label-caps">New token created (copy now, won't be shown again)</span>
            <div className="flex items-center gap-2">
              <code className="text-indigo-300 font-code-snippet text-xs truncate break-all">{apiToken}</code>
              <CopyButton value={apiToken} copiedKey={copied} setCopiedKey={setCopied} copyKey="api-token" />
            </div>
          </div>
        )}

        {apiTokensList.length > 0 && (
          <div className="space-y-1.5 pt-2">
            <div className="flex items-center justify-between px-1">
              <p className="text-[10px] text-slate-500 font-label-caps">Active Tokens ({apiTokensList.length})</p>
              {loadingTokens && <RefreshCw size={10} className="text-slate-500 animate-spin" />}
            </div>
            {apiTokensList.map((t) => (
              <div key={t.id} className="flex items-center justify-between gap-2 bg-slate-900/40 px-3 py-2 rounded-lg border border-slate-800/50 text-[11px]">
                <code className="text-slate-400 font-code-snippet truncate">...{t.id.slice(-8)}</code>
                <div className="flex items-center gap-3">
                  <span className="text-slate-500">{new Date(t.created_at).toLocaleDateString()}</span>
                  <button onClick={() => revokeToken(t.id)} className="text-slate-500 hover:text-red-400 transition-colors uppercase font-bold text-[9px]">
                    Revoke
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </Panel>

      <Panel
        title="Configured URLs"
        subtitle="Each listener can mint a fresh secret-backed ingress URL using the short {userkey}.{secret} format."
        action={
          <button
            onClick={() => onRefresh().catch((err) => setError(err.message))}
            className="text-slate-400 hover:text-white"
            title="Refresh listeners"
          >
            <RefreshCw size={16} />
          </button>
        }
      >
        {loadingSecrets && <InlineNotice>Refreshing listener secrets and URL placeholders...</InlineNotice>}
        <div className="space-y-3">
          {listeners.map((listener) => {
            const key = `${listener.provider}:${listener.listener_id}`;
            const createdSecret = secretMap[key];
            const history = secretsHistory[key] || [];
            const latestBackendSecret = history[0];
            const mintedURL = createdSecret?.webhook_url || latestBackendSecret?.webhook_url || listenerIngressTemplate(listener, publicAlias);
            const mintedWebhookID = createdSecret?.webhook_id || latestBackendSecret?.webhook_id || listenerWebhookIDTemplate(listener, publicAlias);
            const draft = secretComposer[key] || { mode: 'auto', secret_value: '' };

            return (
              <div key={key} className="rounded-2xl border border-slate-800 bg-slate-950/30 p-4 space-y-3">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <p className="text-white text-sm font-medium">
                      {listener.listener_display_name || `${listener.provider} · ${listener.listener_id}`}
                    </p>
                    <p className="text-slate-500 text-[11px] mt-1">
                      {listener.deployment_mode} · {listener.type_key}
                    </p>
                  </div>
                  <div className="flex items-start gap-2">
                    <StatusBadge status="ACTIVE" />
                    <button
                      onClick={() => deleteListener(listener)}
                      title="Delete listener"
                      className="ml-1 text-slate-500 hover:text-red-400 transition-colors"
                    >
                      <svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><polyline points="3 6 5 6 21 6" /><path d="M19 6l-1 14H6L5 6" /><path d="M10 11v6" /><path d="M14 11v6" /><path d="M9 6V4h6v2" /></svg>
                    </button>
                  </div>
                </div>

                <div className="space-y-2">
                  <div className="flex items-center gap-2 bg-slate-950/60 px-3 py-2 rounded-lg border border-slate-800">
                    <code className="text-indigo-300 font-code-snippet text-xs break-all">{mintedURL}</code>
                    <CopyButton
                      value={mintedURL}
                      copiedKey={copied}
                      setCopiedKey={setCopied}
                      copyKey={`listener-${listener.listener_id}`}
                      title="Copy primary URL"
                    />
                  </div>
                  <div className="flex items-center gap-2 bg-slate-950/30 px-3 py-2 rounded-lg border border-slate-800/70">
                    <code className="text-slate-300 font-code-snippet text-[11px] break-all">{mintedWebhookID}</code>
                    <CopyButton
                      value={mintedWebhookID}
                      copiedKey={copied}
                      setCopiedKey={setCopied}
                      copyKey={`listener-id-${listener.listener_id}`}
                      title="Copy inbox address"
                    />
                  </div>
                  <p className="px-1 text-[10px] text-slate-500">This webhook ID also works as the mailbox address for the separate SES mail ingress service.</p>

                  {(createdSecret?.secret_value || latestBackendSecret?.secret_value) && (
                    <div className="flex items-center gap-2 bg-slate-950/30 px-3 py-2 rounded-lg border border-slate-800/70">
                      <code className="text-emerald-300 font-code-snippet text-[11px] break-all">
                        {createdSecret?.secret_value || latestBackendSecret?.secret_value}
                      </code>
                      <CopyButton
                        value={createdSecret?.secret_value || latestBackendSecret?.secret_value}
                        copiedKey={copied}
                        setCopiedKey={setCopied}
                        copyKey={`listener-secret-${listener.listener_id}`}
                        title="Copy raw secret"
                      />
                    </div>
                  )}

                  <details className="rounded-xl border border-slate-800 bg-slate-950/40 px-3 py-2">
                    <summary className="cursor-pointer text-[11px] text-slate-400">Legacy compatibility URLs</summary>
                    <div className="mt-2 space-y-2 text-[11px]">
                      <div>
                        <div className="text-slate-500 mb-1">Provider-aware ingest URL</div>
                        <code className="text-slate-300 break-all">{createdSecret?.ingest_webhook_url || latestBackendSecret?.ingest_webhook_url || listener.ingest_webhook_url_template || `https://app.agenthook.store/ingest/${accountSlug}/${listener.provider}/${listener.listener_id}/[secret]`}</code>
                      </div>
                      <div>
                        <div className="text-slate-500 mb-1">Legacy type-key URL</div>
                        <code className="text-slate-300 break-all">{createdSecret?.legacy_webhook_url || latestBackendSecret?.legacy_webhook_url || listener.legacy_webhook_url}</code>
                      </div>
                    </div>
                  </details>

                  {history.length > 0 && (
                    <div className="space-y-1.5 pt-1">
                      <p className="text-[10px] text-slate-500 font-label-caps px-1">Other Active Secrets</p>
                      {history.map((s) => (
                        <div key={s.id} className="bg-slate-900/40 px-3 py-1.5 rounded-lg border border-slate-800/50 text-[10px] space-y-1">
                          <code className="text-slate-400 block break-all">{s.webhook_url}</code>
                          <div className="flex items-center justify-between gap-2">
                            <code className="text-slate-500 break-all">{s.webhook_id}</code>
                            <span className="text-slate-600 shrink-0">{new Date(s.created_at).toLocaleDateString()}</span>
                          </div>
                          {s.secret_value && (
                            <div className="flex items-center gap-2">
                              <code className="text-emerald-300 break-all">{s.secret_value}</code>
                              <CopyButton
                                value={s.secret_value}
                                copiedKey={copied}
                                setCopiedKey={setCopied}
                                copyKey={`listener-history-secret-${s.id}`}
                                title="Copy historical raw secret"
                              />
                            </div>
                          )}
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                <div className="space-y-2">
                  <div className="grid grid-cols-2 gap-2">
                    <button
                      type="button"
                      onClick={() => setSecretComposer((current) => ({ ...current, [key]: { ...draft, mode: 'auto' } }))}
                      className={`rounded-lg border px-3 py-2 text-xs font-semibold ${draft.mode === 'auto' ? 'border-primary bg-primary/10 text-white' : 'border-slate-800 text-slate-400'}`}
                    >
                      Generate automatically
                    </button>
                    <button
                      type="button"
                      onClick={() => setSecretComposer((current) => ({ ...current, [key]: { ...draft, mode: 'manual' } }))}
                      className={`rounded-lg border px-3 py-2 text-xs font-semibold ${draft.mode === 'manual' ? 'border-primary bg-primary/10 text-white' : 'border-slate-800 text-slate-400'}`}
                    >
                      Set my own secret
                    </button>
                  </div>
                  {draft.mode === 'manual' && (
                    <TextInput
                      value={draft.secret_value}
                      onChange={(e) => setSecretComposer((current) => ({
                        ...current,
                        [key]: { ...draft, secret_value: e.target.value },
                      }))}
                      placeholder="saleslead_2026"
                    />
                  )}
                </div>

                <div className="flex gap-2">
                  <button
                    onClick={() => createSecret(listener)}
                    className="flex-1 bg-slate-900 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform"
                  >
                    Create Secret
                  </button>
                  <button
                    onClick={() => navigator.clipboard.writeText(prettyJSON(listener))}
                    className="px-3 bg-slate-900 border border-slate-800 text-slate-300 rounded-lg text-sm active:scale-95 transition-transform"
                  >
                    JSON
                  </button>
                </div>
              </div>
            );
          })}

          {listeners.length === 0 && (
            <p className="text-slate-500 text-center py-10">
              No specific URLs configured yet. Create your first listener above to unlock secret-backed ingress URLs.
            </p>
          )}
        </div>
      </Panel>
    </motion.div>
  );
};

const SkillsTab = ({ listeners, copied, setCopied, onRefreshListeners }) => {
  const listenerOptions = useMemo(
    () =>
      listeners.map((listener) => ({
        label: listener.listener_display_name || `${listener.provider} · ${listener.listener_id}`,
        value: inferTypeKey(listener),
        listener,
      })),
    [listeners],
  );

  const [selectedTypeKey, setSelectedTypeKey] = useState(listenerOptions[0]?.value || '');
  const [skills, setSkills] = useState([]);
  const [loadingSkills, setLoadingSkills] = useState(false);
  const [allSkillsByType, setAllSkillsByType] = useState({});
  const [skillCounts, setSkillCounts] = useState({});
  const [expandedSkillCard, setExpandedSkillCard] = useState('');
  const [editingSkillID, setEditingSkillID] = useState('');
  const [editingSkillForm, setEditingSkillForm] = useState(null);
  const [savingSkill, setSavingSkill] = useState(false);
  const [skillForm, setSkillForm] = useState({
    skill_key: '',
    skill_prompt: '',
    match_contains: '',
    forced_action: 'store_mysql',
    memory_write_mode: 'update_or_insert',
    priority: 100,
    enabled: true,
  });
  const [skillNotice, setSkillNotice] = useState('');
  const [presetBusy, setPresetBusy] = useState(false);
  const [classifyPayload, setClassifyPayload] = useState('{"provider":"github","event":"push","repository":"agenthook"}');
  const [classifyResult, setClassifyResult] = useState('');
  const [transformPayload, setTransformPayload] = useState('{"provider":"github","event":"push","repository":"agenthook"}');
  const [transformResult, setTransformResult] = useState('');
  const [testBusy, setTestBusy] = useState(false);
  const [recentEvents, setRecentEvents] = useState([]);
  const [loadingRecentEvents, setLoadingRecentEvents] = useState(false);
  const [selectedEventIDs, setSelectedEventIDs] = useState({});
  const [rerunBusy, setRerunBusy] = useState(false);
  const [rerunNotice, setRerunNotice] = useState('');

  const fetchSkillsForType = async (typeKey, includeDisabled = true) => {
    const suffix = includeDisabled ? '&include_disabled=true' : '';
    const data = await apiRequest(`/api/policy/skills?type_key=${encodeURIComponent(typeKey)}${suffix}`);
    return Array.isArray(data) ? data : [];
  };

  useEffect(() => {
    if (!listenerOptions.length) {
      setSelectedTypeKey('');
      setSkillCounts({});
      setSkills([]);
      return;
    }
    if (!listenerOptions.some((item) => item.value === selectedTypeKey)) {
      setSelectedTypeKey(listenerOptions[0].value);
    }
  }, [listenerOptions, selectedTypeKey]);

  useEffect(() => {
    if (!listenerOptions.length) return;
    let cancelled = false;

    Promise.all(
      listenerOptions.map(async (option) => {
        const normalized = await fetchSkillsForType(option.value, true);
        return [option.value, normalized];
      }),
    )
      .then((entries) => {
        if (cancelled) return;
        const nextSkillsByType = Object.fromEntries(entries);
        const nextCounts = Object.fromEntries(entries.map(([typeKey, items]) => [typeKey, items.length]));
        setAllSkillsByType(nextSkillsByType);
        setSkillCounts(nextCounts);

        const firstWithSkills = listenerOptions.find((option) => nextCounts[option.value] > 0)?.value;
        if (
          firstWithSkills &&
          selectedTypeKey === listenerOptions[0]?.value &&
          nextCounts[selectedTypeKey] === 0
        ) {
          setSelectedTypeKey(firstWithSkills);
        }
      })
      .catch((err) => console.error('Failed to prefetch skill counts', err));

    return () => {
      cancelled = true;
    };
  }, [listenerOptions, selectedTypeKey]);

  useEffect(() => {
    if (!selectedTypeKey) return;
    setLoadingSkills(true);
    fetchSkillsForType(selectedTypeKey, true)
      .then((data) => setSkills(data))
      .catch((err) => setSkillNotice(err.message))
      .finally(() => setLoadingSkills(false));
  }, [selectedTypeKey]);

  useEffect(() => {
    if (!selectedTypeKey) {
      setRecentEvents([]);
      setSelectedEventIDs({});
      return;
    }
    setLoadingRecentEvents(true);
    apiRequest('/api/events')
      .then((data) => {
        const normalized = Array.isArray(data) ? data : [];
        setRecentEvents(normalized.filter((event) => event.type_key === selectedTypeKey));
      })
      .catch((err) => setRerunNotice(err.message))
      .finally(() => setLoadingRecentEvents(false));
  }, [selectedTypeKey]);

  const selectedListener = listenerOptions.find((item) => item.value === selectedTypeKey)?.listener || null;

  const createSkill = async () => {
    if (!selectedTypeKey) return;
    setSkillNotice('');
    try {
      const created = await apiRequest('/api/policy/skills', {
        method: 'POST',
        body: JSON.stringify({
          type_key: selectedTypeKey,
          ...skillForm,
          priority: Number(skillForm.priority) || 100,
        }),
      });
      setSkills((current) => [created, ...current]);
      setAllSkillsByType((current) => ({
        ...current,
        [selectedTypeKey]: [created, ...(current[selectedTypeKey] || [])],
      }));
      setSkillCounts((current) => ({
        ...current,
        [selectedTypeKey]: (current[selectedTypeKey] || 0) + 1,
      }));
      setSkillForm((current) => ({
        ...current,
        skill_key: '',
        skill_prompt: '',
        match_contains: '',
      }));
      setSkillNotice('Skill created successfully.');
    } catch (err) {
      setSkillNotice(err.message);
    }
  };

  const applyPreset = async () => {
    if (!selectedListener) return;
    setPresetBusy(true);
    setSkillNotice('');
    try {
      await apiRequest('/v1/presets/webhook-processing', {
        method: 'POST',
        body: JSON.stringify({
          provider: selectedListener.provider,
          listener_id: selectedListener.listener_id,
          specific_prompt: `Handle ${selectedListener.provider} webhook messages for ${selectedListener.listener_id} with concise structured summaries.`,
          specific_match_contains: selectedListener.provider,
          specific_action: 'store_mysql',
          memory_write_mode: 'update_or_insert',
        }),
      });
      const normalized = await fetchSkillsForType(selectedTypeKey, true);
      setSkills(normalized);
      setAllSkillsByType((current) => ({
        ...current,
        [selectedTypeKey]: normalized,
      }));
      setSkillCounts((current) => ({
        ...current,
        [selectedTypeKey]: normalized.length,
      }));
      setSkillNotice('Preset applied. General and provider-specific skills were created.');
      await onRefreshListeners();
    } catch (err) {
      setSkillNotice(err.message);
    } finally {
      setPresetBusy(false);
    }
  };

  const applySkillPack = async (packKey) => {
    if (!selectedTypeKey || !selectedListener) return;
    const pack = SKILL_PACKS[packKey];
    if (!pack) return;
    setPresetBusy(true);
    setSkillNotice('');
    try {
      const existing = await fetchSkillsForType(selectedTypeKey, true);
      const existingKeys = new Set(existing.map((skill) => skill.skill_key));
      const toCreate = pack.skills.filter((skill) => !existingKeys.has(skill.skill_key));
      if (!toCreate.length) {
        setSkillNotice(`${pack.label} is already installed for this listener.`);
        return;
      }
      await Promise.all(
        toCreate.map((skill) =>
          apiRequest('/api/policy/skills', {
            method: 'POST',
            body: JSON.stringify({
              type_key: selectedTypeKey,
              ...skill,
            }),
          }),
        ),
      );
      const normalized = await fetchSkillsForType(selectedTypeKey, true);
      setSkills(normalized);
      setAllSkillsByType((current) => ({
        ...current,
        [selectedTypeKey]: normalized,
      }));
      setSkillCounts((current) => ({
        ...current,
        [selectedTypeKey]: normalized.length,
      }));
      setSkillNotice(`${pack.label} created for ${selectedListener.provider}.`);
    } catch (err) {
      setSkillNotice(err.message);
    } finally {
      setPresetBusy(false);
    }
  };

  const runClassify = async () => {
    setTestBusy(true);
    setClassifyResult('');
    try {
      const result = await apiRequest('/api/resolver/classify', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: classifyPayload,
      });
      setClassifyResult(prettyJSON(result));
    } catch (err) {
      setClassifyResult(prettyJSON({ error: err.message }));
    } finally {
      setTestBusy(false);
    }
  };

  const runTransform = async () => {
    if (!selectedTypeKey) return;
    setTestBusy(true);
    setTransformResult('');
    try {
      const result = await apiRequest(`/api/resolver/transform?type_key=${encodeURIComponent(selectedTypeKey)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: transformPayload,
      });
      setTransformResult(prettyJSON(result));
    } catch (err) {
      setTransformResult(prettyJSON({ error: err.message }));
    } finally {
      setTestBusy(false);
    }
  };

  const listenerCards = listenerOptions.map((option) => ({
    ...option,
    skills: allSkillsByType[option.value] || [],
  }));
  const visibleEvents = recentEvents;
  const selectedVisibleEventIDs = visibleEvents.filter((event) => selectedEventIDs[event.id]).map((event) => event.id);

  const beginEditSkill = (listenerTypeKey, skill) => {
    setExpandedSkillCard(`${listenerTypeKey}:${skill.id}`);
    setEditingSkillID(skill.id);
    setEditingSkillForm({
      skill_key: skill.skill_key,
      skill_prompt: skill.skill_prompt,
      match_contains: skill.match_contains,
      forced_action: skill.forced_action,
      memory_write_mode: skill.memory_write_mode || 'update_or_insert',
      priority: skill.priority || 100,
      enabled: skill.enabled,
    });
  };

  const cancelEditSkill = () => {
    setEditingSkillID('');
    setEditingSkillForm(null);
  };

  const saveSkill = async (typeKey, skillID) => {
    if (!editingSkillForm) return;
    setSavingSkill(true);
    setSkillNotice('');
    try {
      const updated = await apiRequest(`/api/policy/skills/${skillID}`, {
        method: 'PUT',
        body: JSON.stringify({
          type_key: typeKey,
          ...editingSkillForm,
          priority: Number(editingSkillForm.priority) || 100,
        }),
      });
      const nextSkills = (allSkillsByType[typeKey] || []).map((skill) => (skill.id === skillID ? updated : skill));
      setAllSkillsByType((current) => ({ ...current, [typeKey]: nextSkills }));
      if (selectedTypeKey === typeKey) {
        setSkills(nextSkills);
      }
      setSkillCounts((current) => ({ ...current, [typeKey]: nextSkills.length }));
      setSkillNotice('Skill updated successfully.');
      cancelEditSkill();
    } catch (err) {
      setSkillNotice(err.message);
    } finally {
      setSavingSkill(false);
    }
  };

  const toggleEventSelection = (eventID) => {
    setSelectedEventIDs((current) => ({ ...current, [eventID]: !current[eventID] }));
  };

  const toggleAllVisibleEvents = () => {
    const shouldSelectAll = selectedVisibleEventIDs.length !== visibleEvents.length;
    setSelectedEventIDs((current) => {
      const next = { ...current };
      visibleEvents.forEach((event) => {
        next[event.id] = shouldSelectAll;
      });
      return next;
    });
  };

  const rerunSelectedEvents = async () => {
    if (!selectedVisibleEventIDs.length) return;
    setRerunBusy(true);
    setRerunNotice('');
    try {
      await Promise.all(selectedVisibleEventIDs.map((eventID) => apiRequest(`/api/events/${eventID}/re-run`, { method: 'POST' })));
      const refreshed = await apiRequest('/api/events');
      const normalized = Array.isArray(refreshed) ? refreshed : [];
      setRecentEvents(normalized.filter((event) => event.type_key === selectedTypeKey));
      setSelectedEventIDs({});
      setRerunNotice(`Reclassified ${selectedVisibleEventIDs.length} message${selectedVisibleEventIDs.length === 1 ? '' : 's'}.`);
    } catch (err) {
      setRerunNotice(err.message);
    } finally {
      setRerunBusy(false);
    }
  };

  return (
    <motion.div
      initial={{ opacity: 0, x: -20 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: 20 }}
      className="space-y-4"
    >
      <h2 className="px-1 text-white">Skills</h2>

      {!listenerOptions.length && (
        <Panel
          title="No listener selected yet"
          subtitle="Create a listener first. Skills are attached per webhook type, so the UI needs at least one listener to target."
          action={<Brain size={18} className="text-primary" />}
        >
          <InlineNotice>Open the URLs tab, create a listener, then come back here to attach routing skills and test payload behavior.</InlineNotice>
        </Panel>
      )}

      {listenerOptions.length > 0 && (
        <>
          <Panel
            title="Skill Target"
            subtitle="Pick the webhook type you want to enrich with prompt-based routing or provider-specific handling."
            action={<Sparkles size={18} className="text-primary" />}
          >
            <FormField label="Listener / Type">
              <Select value={selectedTypeKey} onChange={(e) => setSelectedTypeKey(e.target.value)}>
                {listenerOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {skillCounts[option.value] > 0 ? `${option.label} (${skillCounts[option.value]} skills)` : option.label}
                  </option>
                ))}
              </Select>
            </FormField>
            {selectedListener && (
              <div className="flex items-center gap-2 bg-slate-950/50 px-3 py-2 rounded-lg border border-slate-800">
                <code className="text-indigo-300 font-code-snippet text-xs break-all">{selectedTypeKey}</code>
                <CopyButton
                  value={selectedTypeKey}
                  copiedKey={copied}
                  setCopiedKey={setCopied}
                  copyKey="selected-type-key"
                  title="Copy type key"
                />
              </div>
            )}
          </Panel>

          {Object.values(skillCounts).some((count) => count > 0) && skillCounts[selectedTypeKey] === 0 && (
            <InlineNotice>
              This listener has no saved skills. The selector above highlights listeners that already do.
            </InlineNotice>
          )}

          <Panel
            title="Bootstrap Provider Skills"
            subtitle="Create a sensible baseline automatically, then layer in channel-specific packs like WhatsApp, email, or GetApproval."
            action={<Wand2 size={18} className="text-primary" />}
          >
            {skillNotice && (
              <InlineNotice tone={skillNotice.toLowerCase().includes('success') || skillNotice.toLowerCase().includes('preset') ? 'success' : 'info'}>
                {skillNotice}
              </InlineNotice>
            )}
            <button
              onClick={applyPreset}
              disabled={presetBusy || !selectedListener}
              className="w-full bg-slate-900 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
            >
              {presetBusy ? 'APPLYING...' : 'APPLY WEBHOOK PROCESSING PRESET'}
            </button>
            <div className="grid grid-cols-1 gap-2 pt-2">
              <button
                onClick={() => applySkillPack('whatsapp')}
                disabled={presetBusy || !selectedListener}
                className="w-full flex items-center justify-center gap-2 bg-slate-950/60 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
              >
                <MessageSquareQuote size={16} />
                {presetBusy ? 'APPLYING...' : 'CREATE WHATSAPP SKILLS'}
              </button>
              <button
                onClick={() => applySkillPack('email')}
                disabled={presetBusy || !selectedListener}
                className="w-full flex items-center justify-center gap-2 bg-slate-950/60 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
              >
                <Mail size={16} />
                {presetBusy ? 'APPLYING...' : 'CREATE EMAIL SKILLS'}
              </button>
              <button
                onClick={() => applySkillPack('gate')}
                disabled={presetBusy || !selectedListener}
                className="w-full flex items-center justify-center gap-2 bg-slate-950/60 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
              >
                <BadgeCheck size={16} />
                {presetBusy ? 'APPLYING...' : 'CREATE GETAPPROVAL SKILLS'}
              </button>
            </div>
          </Panel>

          <Panel
            title="Create Skill"
            subtitle="Attach a prompt, a match rule, and a forced action to a specific webhook type."
            action={<Brain size={18} className="text-primary" />}
          >
            <div className="grid grid-cols-1 gap-3">
              <FormField label="Skill Key" hint="Use a stable identifier like github-priority-triage.">
                <TextInput
                  value={skillForm.skill_key}
                  onChange={(e) => setSkillForm((current) => ({ ...current, skill_key: e.target.value }))}
                  placeholder="github-priority-triage"
                />
              </FormField>
              <FormField label="Skill Prompt">
                <TextArea
                  value={skillForm.skill_prompt}
                  onChange={(e) => setSkillForm((current) => ({ ...current, skill_prompt: e.target.value }))}
                  placeholder="Summarize important repository changes and flag deploy-impacting messages."
                />
              </FormField>
              <FormField label="Match Contains" hint="Optional substring check before this skill becomes relevant.">
                <TextInput
                  value={skillForm.match_contains}
                  onChange={(e) => setSkillForm((current) => ({ ...current, match_contains: e.target.value }))}
                  placeholder="deploy"
                />
              </FormField>
              <div className="grid grid-cols-2 gap-3">
                <FormField label="Forced Action">
                  <Select
                    value={skillForm.forced_action}
                    onChange={(e) => setSkillForm((current) => ({ ...current, forced_action: e.target.value }))}
                  >
                    {FORCED_ACTION_OPTIONS.map((item) => (
                      <option key={item} value={item}>
                        {item}
                      </option>
                    ))}
                  </Select>
                </FormField>
                <FormField label="Memory Write Mode">
                  <Select
                    value={skillForm.memory_write_mode}
                    onChange={(e) => setSkillForm((current) => ({ ...current, memory_write_mode: e.target.value }))}
                  >
                    {MEMORY_WRITE_MODES.map((item) => (
                      <option key={item} value={item}>
                        {item}
                      </option>
                    ))}
                  </Select>
                </FormField>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <FormField label="Priority">
                  <TextInput
                    type="number"
                    value={skillForm.priority}
                    onChange={(e) => setSkillForm((current) => ({ ...current, priority: e.target.value }))}
                  />
                </FormField>
                <FormField label="Enabled">
                  <Select
                    value={String(skillForm.enabled)}
                    onChange={(e) => setSkillForm((current) => ({ ...current, enabled: e.target.value === 'true' }))}
                  >
                    <option value="true">Enabled</option>
                    <option value="false">Disabled</option>
                  </Select>
                </FormField>
              </div>
              <button
                onClick={createSkill}
                className="w-full bg-primary text-on-primary font-bold py-2 rounded-lg text-sm active:scale-95 transition-transform"
              >
                CREATE SKILL
              </button>
            </div>
          </Panel>

          <Panel
            title="All Skills"
            subtitle="Every listener's saved skills are shown below. Click a card to expand the full rule."
            action={loadingSkills ? <RefreshCw size={16} className="text-primary animate-spin" /> : null}
          >
            <div className="space-y-3">
              {listenerCards.map((listenerCard) => (
                <div key={listenerCard.value} className="space-y-2">
                  <div className="flex items-center justify-between px-1">
                    <div>
                      <p className="text-white text-sm font-medium">{listenerCard.label}</p>
                      <p className="text-slate-500 text-[11px] mt-1">{listenerCard.skills.length} saved skill{listenerCard.skills.length === 1 ? '' : 's'}</p>
                    </div>
                    {listenerCard.value === selectedTypeKey && <StatusBadge status="ACTIVE" />}
                  </div>

                  {listenerCard.skills.map((skill) => {
                    const cardID = `${listenerCard.value}:${skill.id}`;
                    const isExpanded = expandedSkillCard === cardID;
                    const isEditing = editingSkillID === skill.id && editingSkillForm;
                    return (
                      <div
                        key={skill.id}
                        onClick={() => setExpandedSkillCard((current) => (current === cardID ? '' : cardID))}
                        className="w-full text-left rounded-2xl border border-slate-800 bg-slate-950/30 p-4 space-y-3 transition-colors hover:border-slate-700 cursor-pointer"
                        role="button"
                        tabIndex={0}
                        onKeyDown={(event) => {
                          if (event.target !== event.currentTarget) {
                            return;
                          }
                          if (event.key === 'Enter' || event.key === ' ') {
                            event.preventDefault();
                            setExpandedSkillCard((current) => (current === cardID ? '' : cardID));
                          }
                        }}
                      >
                        <div className="flex items-start justify-between gap-3">
                          <div>
                            <p className="text-white text-sm font-medium">{skill.skill_key}</p>
                            <p className="text-slate-500 text-[11px] mt-1">
                              {skill.match_contains || 'matches all'} · action {skill.forced_action || 'auto'} · priority {skill.priority}
                            </p>
                          </div>
                          <div className="flex items-center gap-2">
                            <StatusBadge status={skill.enabled ? 'ACTIVE' : 'LEARNING'} />
                            {isExpanded ? <ChevronUp size={16} className="text-slate-500" /> : <ChevronDown size={16} className="text-slate-500" />}
                          </div>
                        </div>
                        {isExpanded && (
                          <div className="space-y-2 border-t border-slate-800/80 pt-3">
                            {!isEditing && (
                              <>
                                <p className="text-slate-300 text-sm">{skill.skill_prompt || 'No prompt text saved.'}</p>
                                <div className="grid grid-cols-1 gap-2 text-[11px] text-slate-400">
                                  <div>Type key: <code className="text-slate-300 break-all">{listenerCard.value}</code></div>
                                  <div>Memory write mode: <span className="text-slate-300">{skill.memory_write_mode || 'default'}</span></div>
                                  <div>Created: <span className="text-slate-300">{skill.created_at ? new Date(skill.created_at).toLocaleString() : 'Unknown'}</span></div>
                                </div>
                                <div className="flex gap-2 pt-1">
                                  <button
                                    type="button"
                                    onClick={(event) => {
                                      event.stopPropagation();
                                      beginEditSkill(listenerCard.value, skill);
                                    }}
                                    className="px-3 py-1.5 rounded-lg border border-slate-700 text-xs text-slate-200 hover:bg-slate-900"
                                  >
                                    Edit Skill
                                  </button>
                                </div>
                              </>
                            )}
                            {isEditing && (
                              <div
                                className="space-y-3"
                                onClick={(event) => event.stopPropagation()}
                              >
                                <FormField label="Skill Key">
                                  <TextInput
                                    value={editingSkillForm.skill_key}
                                    onChange={(event) => setEditingSkillForm((current) => ({ ...current, skill_key: event.target.value }))}
                                  />
                                </FormField>
                                <FormField label="Skill Prompt">
                                  <TextArea
                                    value={editingSkillForm.skill_prompt}
                                    onChange={(event) => setEditingSkillForm((current) => ({ ...current, skill_prompt: event.target.value }))}
                                  />
                                </FormField>
                                <FormField label="Match Contains">
                                  <TextInput
                                    value={editingSkillForm.match_contains}
                                    onChange={(event) => setEditingSkillForm((current) => ({ ...current, match_contains: event.target.value }))}
                                  />
                                </FormField>
                                <div className="grid grid-cols-2 gap-3">
                                  <FormField label="Forced Action">
                                    <Select
                                      value={editingSkillForm.forced_action}
                                      onChange={(event) => setEditingSkillForm((current) => ({ ...current, forced_action: event.target.value }))}
                                    >
                                      {FORCED_ACTION_OPTIONS.map((item) => (
                                        <option key={item} value={item}>
                                          {item}
                                        </option>
                                      ))}
                                    </Select>
                                  </FormField>
                                  <FormField label="Memory Write Mode">
                                    <Select
                                      value={editingSkillForm.memory_write_mode}
                                      onChange={(event) => setEditingSkillForm((current) => ({ ...current, memory_write_mode: event.target.value }))}
                                    >
                                      {MEMORY_WRITE_MODES.map((item) => (
                                        <option key={item} value={item}>
                                          {item}
                                        </option>
                                      ))}
                                    </Select>
                                  </FormField>
                                </div>
                                <div className="grid grid-cols-2 gap-3">
                                  <FormField label="Priority">
                                    <TextInput
                                      type="number"
                                      value={editingSkillForm.priority}
                                      onChange={(event) => setEditingSkillForm((current) => ({ ...current, priority: event.target.value }))}
                                    />
                                  </FormField>
                                  <FormField label="Enabled">
                                    <Select
                                      value={String(editingSkillForm.enabled)}
                                      onChange={(event) => setEditingSkillForm((current) => ({ ...current, enabled: event.target.value === 'true' }))}
                                    >
                                      <option value="true">Enabled</option>
                                      <option value="false">Disabled</option>
                                    </Select>
                                  </FormField>
                                </div>
                                <div className="flex gap-2">
                                  <button
                                    type="button"
                                    onClick={() => saveSkill(listenerCard.value, skill.id)}
                                    disabled={savingSkill}
                                    className="px-3 py-1.5 rounded-lg bg-primary text-on-primary text-xs font-semibold disabled:opacity-50"
                                  >
                                    {savingSkill ? 'Saving...' : 'Save'}
                                  </button>
                                  <button
                                    type="button"
                                    onClick={cancelEditSkill}
                                    className="px-3 py-1.5 rounded-lg border border-slate-700 text-xs text-slate-200 hover:bg-slate-900"
                                  >
                                    Cancel
                                  </button>
                                </div>
                              </div>
                            )}
                          </div>
                        )}
                      </div>
                    );
                  })}

                  {!listenerCard.skills.length && (
                    <p className="text-slate-500 text-center py-4 rounded-2xl border border-dashed border-slate-800">
                      No skills saved for this listener yet.
                    </p>
                  )}
                </div>
              ))}
              {!listenerCards.some((listenerCard) => listenerCard.skills.length > 0) && !loadingSkills && (
                <p className="text-slate-500 text-center py-6">
                  No skills exist for this account yet. Use the preset or create one manually.
                </p>
              )}
            </div>
          </Panel>

          <Panel
            title="Existing Messages"
            subtitle="Select one or more recent messages for this listener and reclassify them with the latest skill rules."
            action={loadingRecentEvents ? <RefreshCw size={16} className="text-primary animate-spin" /> : null}
          >
            {rerunNotice && (
              <InlineNotice tone={rerunNotice.toLowerCase().includes('reclassified') ? 'success' : 'info'}>
                {rerunNotice}
              </InlineNotice>
            )}
            {!!visibleEvents.length && (
              <div className="flex items-center justify-between gap-3">
                <label className="flex items-center gap-2 text-xs text-slate-300">
                  <input
                    type="checkbox"
                    checked={visibleEvents.length > 0 && selectedVisibleEventIDs.length === visibleEvents.length}
                    onChange={toggleAllVisibleEvents}
                  />
                  Select all visible
                </label>
                <button
                  onClick={rerunSelectedEvents}
                  disabled={rerunBusy || selectedVisibleEventIDs.length === 0}
                  className="px-3 py-1.5 rounded-lg bg-slate-900 border border-slate-800 text-white text-xs font-semibold disabled:opacity-50"
                >
                  {rerunBusy ? 'Reclassifying...' : `Reclassify Selected (${selectedVisibleEventIDs.length})`}
                </button>
              </div>
            )}
            <div className="space-y-2">
              {visibleEvents.map((event) => (
                <label key={event.id} className="flex items-start gap-3 rounded-2xl border border-slate-800 bg-slate-950/30 p-3">
                  <input
                    type="checkbox"
                    checked={!!selectedEventIDs[event.id]}
                    onChange={() => toggleEventSelection(event.id)}
                    className="mt-1"
                  />
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex items-center justify-between gap-2">
                      <span className="text-[11px] text-slate-500">{new Date(event.created_at).toLocaleString()}</span>
                      <StatusBadge status={event.status || 'ACTIVE'} />
                    </div>
                    <p className="text-[11px] text-slate-400">Action: {event.action_selected || 'unknown'}</p>
                    <pre className="whitespace-pre-wrap break-words font-code-snippet text-[11px] leading-relaxed text-slate-300">
                      {payloadPreview(event) || `Event ${event.id}`}
                    </pre>
                  </div>
                </label>
              ))}
              {!visibleEvents.length && !loadingRecentEvents && (
                <p className="text-slate-500 text-center py-6">
                  No recent messages found for this listener yet.
                </p>
              )}
            </div>
          </Panel>

          <Panel
            title="Test Skills"
            subtitle="Dry-run classification and transformation with a sample payload before wiring the listener into production traffic."
            action={<TestTube2 size={18} className="text-primary" />}
          >
            <div className="space-y-4">
              <FormField label="Classifier Payload">
                <TextArea value={classifyPayload} onChange={(e) => setClassifyPayload(e.target.value)} />
              </FormField>
              <button
                onClick={runClassify}
                disabled={testBusy}
                className="w-full bg-slate-900 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
              >
                {testBusy ? 'RUNNING...' : 'RUN CLASSIFY DRY-RUN'}
              </button>
              {classifyResult && <pre className="text-xs text-slate-300 bg-slate-950/80 p-3 rounded-xl overflow-auto border border-slate-800">{classifyResult}</pre>}

              <FormField label="Transform Payload">
                <TextArea value={transformPayload} onChange={(e) => setTransformPayload(e.target.value)} />
              </FormField>
              <button
                onClick={runTransform}
                disabled={testBusy || !selectedTypeKey}
                className="w-full bg-slate-900 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
              >
                {testBusy ? 'RUNNING...' : 'RUN TRANSFORM DRY-RUN'}
              </button>
              {transformResult && <pre className="text-xs text-slate-300 bg-slate-950/80 p-3 rounded-xl overflow-auto border border-slate-800">{transformResult}</pre>}
            </div>
          </Panel>
        </>
      )}
    </motion.div>
  );
};

const StatusBadge = ({ status }) => {
  const configs = {
    ACTIVE: { color: 'text-stage-active', bg: 'bg-stage-active/10', border: 'border-stage-active/20' },
    SHADOW: { color: 'text-stage-shadow', bg: 'bg-stage-shadow/10', border: 'border-stage-shadow/20' },
    LEARNING: { color: 'text-stage-learning', bg: 'bg-stage-learning/10', border: 'border-stage-learning/20', pulse: true },
  };

  const config = configs[status] || configs.ACTIVE;

  return (
    <div className={`${config.bg} ${config.color} text-[10px] font-bold px-2 py-0.5 rounded-full border ${config.border} flex items-center gap-1 shrink-0`}>
      {config.pulse && <span className="w-1.5 h-1.5 bg-stage-learning rounded-full animate-pulse" />}
      {status}
    </div>
  );
};

export default App;
