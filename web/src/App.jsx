import React, { useEffect, useMemo, useState } from 'react';
import { TopAppBar } from './components/TopAppBar';
import { SideDrawer } from './components/SideDrawer';
import { Metrics } from './components/Metrics';
import { StoryboardCard } from './components/StoryboardCard';
import {
  Plus,
  RefreshCw,
  Copy,
  Check,
  CalendarDays,
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
  BookOpen,
  ArrowUpRight,
  Save,
  Trash2,
  ShieldCheck,
  Activity,
} from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';
import { useAuth } from './context/AuthContext';

const VALID_TABS = new Set(['home', 'heartbeat', 'storyboard', 'skills', 'integrations', 'integration-secrets', 'urls', 'api-tokens', 'enterprise', 'docs', 'byok']);

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
const DEFAULT_APP_PROFILE = {
  plan: 'basic',
  deployment_mode: 'multitenant',
  docs_path: '/app?tab=docs',
  home_docs_anchor: '/#docs',
};
const ENTERPRISE_SETUP_PRICE = import.meta.env.VITE_ENTERPRISE_SETUP_PRICE || '200';
const ENTERPRISE_CAL_URL = import.meta.env.VITE_ENTERPRISE_CAL_URL || 'https://cal.com/abhinava-hiddentalentclub/agenthook-enterprise';

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

function enterprisePrefillURL(baseURL, form) {
  const params = new URLSearchParams();
  if (form.name.trim()) params.set('name', form.name.trim());
  if (form.email.trim()) params.set('email', form.email.trim());
  const notes = [form.company.trim(), form.useCase.trim()].filter(Boolean).join(' | ');
  if (notes) params.set('notes', notes);
  const query = params.toString();
  return query ? `${baseURL}${baseURL.includes('?') ? '&' : '?'}${query}` : baseURL;
}

const PlanComparison = () => {
  const features = [
    { name: 'Setup Cost', free: '$0', enterprise: '$1,500 (One-time)' },
    { name: 'Monthly Subscription', free: '$0/mo', enterprise: '$0/mo' },
    { name: 'Deployment', free: 'Shared Multi-tenant', enterprise: 'Private Single-tenant (AWS/Cloud)' },
    { name: 'Inbound Webhooks', free: 'Up to 20', enterprise: 'Unlimited' },
    { name: 'Inbound Emails', free: 'Up to 20', enterprise: 'Unlimited' },
    { name: 'Monthly Messages', free: 'Up to 3,000', enterprise: 'Unlimited' },
    { name: 'Outbound Emails', free: 'No (Receive Only)', enterprise: 'Full Support' },
    { name: 'Domain', free: 'agenthook.store', enterprise: 'Your Own Custom Domain' },
    { name: 'Data Privacy', free: 'Debugging Access', enterprise: 'Total Isolation' },
  ];

  return (
    <div className="overflow-x-auto rounded-3xl border border-slate-800 bg-slate-950/40 p-1">
      <table className="w-full text-left text-sm">
        <thead>
          <tr className="border-b border-slate-800 bg-slate-900/50">
            <th className="px-4 py-3 font-semibold text-slate-400">Feature</th>
            <th className="px-4 py-3 font-semibold text-indigo-400">Free Plan</th>
            <th className="px-4 py-3 font-semibold text-amber-400">Enterprise</th>
          </tr>
        </thead>
        <tbody>
          {features.map((f, i) => (
            <tr key={f.name} className={i !== features.length - 1 ? 'border-b border-slate-800/50' : ''}>
              <td className="px-4 py-3 font-medium text-slate-200">{f.name}</td>
              <td className="px-4 py-3 text-slate-400">{f.free}</td>
              <td className="px-4 py-3 text-slate-300 font-medium">{f.enterprise}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
};

function EnterpriseSection({ inApp = false, onPrimaryAction }) {
  const [form, setForm] = useState({
    name: '',
    email: '',
    company: '',
    useCase: '',
  });

  const bookingURL = enterprisePrefillURL(ENTERPRISE_CAL_URL, form);

  return (
    <div className="space-y-5">
      <div className="rounded-[32px] border border-amber-400/20 bg-[linear-gradient(135deg,rgba(245,158,11,0.16),rgba(15,23,42,0.96))] p-6 md:p-8 space-y-4">
        <div className="flex items-start justify-between gap-4 flex-wrap">
          <div className="space-y-2 max-w-2xl">
            <p className="text-[10px] uppercase tracking-[0.24em] text-amber-300 font-label-caps">Switch to Enterprise</p>
            <h3 className="text-2xl md:text-3xl font-h1 text-white">No monthly subscription. One setup cost of ${ENTERPRISE_SETUP_PRICE}.</h3>
            <p className="text-sm md:text-base text-slate-200">
              Enterprise is for teams that want the whole AgentHook architecture deployed on their own cloud, their own domain,
              and their own operational boundary. We set it up once, wire the domain, webhook ingress, SES mail path, storage,
              and UI, and you run it comfortably from there.
            </p>
          </div>
          <div className="rounded-2xl border border-amber-300/20 bg-slate-950/50 px-4 py-3 text-sm text-slate-100">
            <div className="font-semibold text-white">Enterprise includes</div>
            <div className="mt-1 text-slate-300">Single-tenant deployment, custom domain wiring, AWS/cloud setup, and guided handoff.</div>
          </div>
        </div>

        <div className="grid gap-3 md:grid-cols-4">
          {[
            'Single-tenant deployment path for your company',
            'Your own AWS account or other cloud environment',
            'Webhook, SES mail, storage, and UI setup included',
            'One-time setup cost instead of recurring subscription',
          ].map((item) => (
            <div key={item} className="rounded-2xl border border-slate-800 bg-slate-950/45 px-4 py-4 text-sm text-slate-200">
              {item}
            </div>
          ))}
        </div>
      </div>

      <div className="space-y-4">
        <div className="px-2">
          <p className="text-[10px] uppercase tracking-[0.22em] text-slate-400 font-label-caps">Comparison</p>
          <h4 className="text-xl text-white font-semibold">Free vs. Enterprise</h4>
        </div>
        <PlanComparison />
      </div>

      <div className="grid gap-5 lg:grid-cols-[0.9fr_1.1fr]">
        <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-4">
          <div>
            <p className="text-[10px] uppercase tracking-[0.22em] text-indigo-400 font-label-caps">Request enterprise setup</p>
            <h4 className="text-xl text-white font-semibold">Tell us what you want deployed.</h4>
            <p className="text-sm text-slate-300 mt-1">
              Share the basics, then book an appointment on Cal.com so the deployment plan can be scoped against your domain, cloud, and event volume.
            </p>
          </div>

          <div className="space-y-3">
            <FormField label="Name">
              <TextInput value={form.name} onChange={(e) => setForm((current) => ({ ...current, name: e.target.value }))} placeholder="Abhinav" />
            </FormField>
            <FormField label="Work Email">
              <TextInput value={form.email} onChange={(e) => setForm((current) => ({ ...current, email: e.target.value }))} placeholder="founder@yourcompany.com" />
            </FormField>
            <FormField label="Company or Domain">
              <TextInput value={form.company} onChange={(e) => setForm((current) => ({ ...current, company: e.target.value }))} placeholder="yourcompany.com" />
            </FormField>
            <FormField label="What should we deploy?" hint="Examples: inbound mail, WhatsApp routing, OpenClaw filtering, custom domain, single-tenant AWS setup.">
              <TextArea value={form.useCase} onChange={(e) => setForm((current) => ({ ...current, useCase: e.target.value }))} placeholder="We want AgentHook on our own AWS account with SES mail ingest, a custom domain, and CRM/OpenClaw routing." />
            </FormField>
          </div>

          <div className="flex flex-col sm:flex-row gap-3">
            <a
              href={bookingURL}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center justify-center gap-2 bg-primary text-on-primary px-4 py-3 rounded-2xl font-semibold active:scale-95 transition-transform"
            >
              <CalendarDays size={16} />
              Set appointment on Cal.com
            </a>
            <button
              type="button"
              onClick={onPrimaryAction}
              className="inline-flex items-center justify-center gap-2 border border-slate-700 bg-slate-950/50 px-4 py-3 rounded-2xl font-semibold text-white hover:bg-slate-900 transition-colors"
            >
              <ArrowUpRight size={16} />
              {inApp ? 'Stay in docs' : 'See enterprise benefits'}
            </button>
          </div>
        </div>

        <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-4 space-y-3">
          <div className="flex items-center justify-between gap-3">
            <div>
              <p className="text-[10px] uppercase tracking-[0.22em] text-slate-400 font-label-caps">Cal.com booking</p>
              <h4 className="text-lg text-white font-semibold">Pick a deployment planning slot.</h4>
            </div>
            <a
              href={bookingURL}
              target="_blank"
              rel="noreferrer"
              className="text-sm text-indigo-300 hover:text-indigo-200 inline-flex items-center gap-1"
            >
              Open full page
              <ArrowUpRight size={14} />
            </a>
          </div>
          <div className="overflow-hidden rounded-2xl border border-slate-800 bg-slate-950">
            <iframe
              title="Enterprise booking calendar"
              src={bookingURL}
              className="w-full min-h-[760px] bg-white"
              loading="lazy"
              referrerPolicy="no-referrer-when-downgrade"
            />
          </div>
        </div>
      </div>
    </div>
  );
}

function EnterpriseBanner({ onClick }) {
  return (
    <button
      onClick={onClick}
      className="w-full relative z-50 bg-[linear-gradient(135deg,rgba(245,158,11,0.92),rgba(251,191,36,0.82))] hover:bg-[linear-gradient(135deg,rgba(245,158,11,1),rgba(251,191,36,1))] px-4 py-2 text-center transition-colors flex flex-col sm:flex-row items-center justify-center gap-1 sm:gap-3 group"
      title="Switch to Enterprise"
    >
      <div className="flex items-center gap-2">
        <CalendarDays size={14} className="text-amber-950" />
        <span className="text-[11px] sm:text-xs font-bold text-amber-950 uppercase tracking-[0.1em]">Switch to Enterprise:</span>
      </div>
      <span className="text-xs sm:text-sm text-amber-950/90 font-medium group-hover:text-amber-950 transition-colors">
        No monthly subscription. One setup cost of ${ENTERPRISE_SETUP_PRICE}.
      </span>
      <span className="text-xs text-amber-950/70 underline group-hover:text-amber-950 ml-1">Learn more &rarr;</span>
    </button>
  );
}

function normalizeAppProfile(profile) {
  return {
    ...DEFAULT_APP_PROFILE,
    ...(profile || {}),
    plan: profile?.plan === 'enterprise' ? 'enterprise' : 'basic',
    deployment_mode: profile?.deployment_mode === 'single_tenant' ? 'single_tenant' : 'multitenant',
  };
}

function DocsContent({ appProfile, login, inApp = false, onOpenEnterprise }) {
  const planLabel = appProfile.plan === 'enterprise' ? 'Enterprise' : 'Basic';
  const deploymentLabel = appProfile.deployment_mode === 'single_tenant' ? 'Single-tenant' : 'Multitenant';

  return (
    <div className="space-y-6">
      <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-3">
        <div className="flex items-center justify-between gap-3 flex-wrap">
          <div>
            <p className="text-[10px] uppercase tracking-[0.22em] text-indigo-400 font-label-caps">Docs Overview</p>
            <h3 className="text-xl text-white font-semibold">One guide for humans, agents, webhooks, and mailbox-style ingress.</h3>
          </div>
          <div className="flex gap-2 text-[10px] uppercase tracking-[0.18em]">
            <span className="rounded-full border border-slate-700 bg-slate-900/80 px-3 py-1 text-slate-300">{planLabel} plan</span>
            <span className="rounded-full border border-slate-700 bg-slate-900/80 px-3 py-1 text-slate-300">{deploymentLabel}</span>
          </div>
        </div>
        <p className="text-sm text-slate-300">
          This deployment runs the current product in the <span className="text-white font-semibold">Basic</span> plan and
          uses <span className="text-white font-semibold">multitenant</span> routing. Enterprise-only behavior and
          single-tenant deployment decisions are controlled by environment variables at deploy time, not by per-listener UI choices here.
        </p>
        <p className="text-sm text-slate-300">
          You can also deploy the full architecture on your own AWS account or any cloud you prefer, bind it to the domain you want,
          and run it comfortably as your own stack. That deployment path is intended as a <span className="text-white font-semibold">one-time setup plan</span>,
          not a recurring platform charge.
        </p>
      </div>

      <div className="grid gap-4 lg:grid-cols-[1.05fr_0.95fr]">
        <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-3">
          <p className="text-[10px] uppercase tracking-[0.22em] text-indigo-400 font-label-caps">Agent-readable contract</p>
          <p className="text-sm text-slate-300">
            This section is intentionally compact and explicit so an LLM can ingest it with minimal ambiguity.
          </p>
          <pre className="overflow-auto rounded-2xl border border-slate-800 bg-slate-950 p-4 text-[12px] text-slate-200 font-code-snippet whitespace-pre-wrap">{`AGENTHOOK_DEPLOYMENT
- plan: ${appProfile.plan}
- deployment_mode: ${appProfile.deployment_mode}
- public_base_url: https://app.agenthook.store

INGRESS_MODES
1. HTTP webhook
   POST /{public_alias}.{secret}
   body: JSON payload

2. Email ingress
   {public_alias}.{secret}@app.agenthook.store
   path: SES -> S3 -> mail ingress Lambda -> AgentHook event

PRIMARY OBJECTS
- listener: provider + listener_id + deployment_mode + type_key
- secret: activates both webhook URL and inbox address
- skill: classifies, routes, tags, summarizes, or nominates actions
- integration: named target like OpenClaw or any forward URL
- event: stored storyboard item with payload, processed_text, action, tags

BASIC PLAN CAPABILITIES
- create listeners
- create reusable secrets
- ingest HTTP or email events
- classify and reclassify events
- manage skills, integrations, BYOK, and integration secrets
- forward important events to downstream systems

CURRENT DEPLOYMENT RULES
- deployment mode is fixed by env for this deployment
- current mode: ${appProfile.deployment_mode}
- current UI should be treated as multitenant only
- enterprise behavior is enabled only when APP_PLAN=enterprise

OPERATOR API BASICS
- GET /api/app-profile
- GET /api/me
- GET /v1/listeners
- POST /v1/listeners
- GET /v1/listeners/{listener_id}/secrets?provider={provider}
- POST /api/events/{event_id}/re-run
- GET /api/policy/skills?type_key={type_key}
- GET /api/forward-targets

EXPECTED EVENT FLOW
payload -> preprocess -> deterministic routing -> optional LLM routing -> selected skill(s) -> action/integration -> storyboard storage`}</pre>
        </div>

        <div className="space-y-4">
          <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-3">
            <p className="text-[10px] uppercase tracking-[0.22em] text-emerald-400 font-label-caps">Human-readable guide</p>
            <p className="text-sm text-slate-300">
              AgentHook sits in front of your noisy systems. It accepts events from apps, email, or internal tools, decides
              what is noise versus signal, stores a clear record in Storyboard, and forwards only meaningful events into your
              downstream workflows.
            </p>
            <p className="text-sm text-slate-300">
              If you want full control, you can deploy this architecture for your own company on AWS or another cloud, connect your own
              domain, and keep the whole stack in your environment. The intent there is a one-time deployment charge rather than an ongoing
              recurring software fee.
            </p>
            <div className="space-y-2 text-sm text-slate-300">
              <p><span className="text-white font-semibold">Webhook magic:</span> every active secret gives you a short URL like <code className="text-indigo-300">https://app.agenthook.store/abhinaviitg18.demo</code>.</p>
              <p><span className="text-white font-semibold">Email magic:</span> the same identity also works as <code className="text-indigo-300">abhinaviitg18.demo@app.agenthook.store</code>.</p>
              <p><span className="text-white font-semibold">In-app magic:</span> Storyboard shows raw or processed content, Skills decide routing, Integrations control side effects, and Reclassify lets you replay past events after improving rules.</p>
            </div>
          </div>

          <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-3">
            <p className="text-[10px] uppercase tracking-[0.22em] text-slate-400 font-label-caps">Examples</p>
            <div className="space-y-3 text-sm text-slate-300">
              <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-3">
                <p className="text-white font-semibold">1. Website lead intake</p>
                <p>Send a JSON payload to the short webhook URL. A lead skill tags it, stores a summary, and forwards the clean payload to OpenClaw or your CRM.</p>
              </div>
              <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-3">
                <p className="text-white font-semibold">2. Email inbox automation</p>
                <p>Email the webhook ID directly. SES receives the raw MIME, the mail Lambda normalizes the message, and AgentHook stores it as an event that skills can triage.</p>
              </div>
              <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-3">
                <p className="text-white font-semibold">3. OpenClaw cost control</p>
                <p>Let AgentHook discard heartbeats and routine status updates before they ever reach OpenClaw, so only high-signal events consume downstream automation.</p>
              </div>
              <div className="rounded-2xl border border-slate-800 bg-slate-900/60 p-3">
                <p className="text-white font-semibold">4. Self-hosted company deployment</p>
                <p>Deploy the same webhook, email, SES, S3, Lambda, and UI architecture on your own AWS account or any cloud, attach your domain, and run it as a one-time setup for your team.</p>
              </div>
            </div>
          </div>

          <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-3">
            <p className="text-[10px] uppercase tracking-[0.22em] text-slate-400 font-label-caps">Getting started</p>
            <ol className="space-y-2 text-sm text-slate-300 list-decimal pl-5">
              <li>Create a listener.</li>
              <li>Copy the short webhook URL or inbox address.</li>
              <li>Add skills and integrations for the specific outcomes you want.</li>
              <li>Send a sample event, then inspect Storyboard and Reclassify if needed.</li>
            </ol>

            <div className="mt-6 pt-4 border-t border-slate-800">
              <p className="text-[10px] uppercase tracking-[0.22em] text-slate-400 font-label-caps mb-2">Claude Skill / System Prompt</p>
              <p className="text-sm text-slate-300 mb-3">
                Download or view the official Claude skill document that explains AgentHook capabilities to an LLM.
              </p>
              <div className="flex flex-col sm:flex-row gap-3">
                <a
                  href="/agenthook-claude-skill.md"
                  download="agenthook-claude-skill.md"
                  className="inline-flex items-center justify-center gap-2 bg-slate-900 border border-slate-700 text-white px-4 py-2 rounded-xl font-semibold hover:bg-slate-800 transition-colors"
                >
                  <Save size={16} />
                  Download Skill (.md)
                </a>
                <a
                  href="/agenthook-claude-skill.md"
                  target="_blank"
                  className="inline-flex items-center justify-center gap-2 bg-slate-950/50 border border-slate-800 text-slate-300 px-4 py-2 rounded-xl hover:text-white hover:border-slate-700 transition-colors"
                >
                  <BookOpen size={16} />
                  View Prompt
                </a>
              </div>
            </div>

            <div className="mt-6 pt-4 border-t border-slate-800">
              <p className="text-[10px] uppercase tracking-[0.22em] text-slate-400 font-label-caps mb-2">Legal & Privacy</p>
              <p className="text-sm text-slate-300 mb-3">
                Review how we handle your data, especially for users on the Free Plan.
              </p>
              <div className="flex flex-col sm:flex-row gap-3">
                <a
                  href="/privacy.md"
                  target="_blank"
                  className="inline-flex items-center justify-center gap-2 bg-slate-950/50 border border-slate-800 text-slate-300 px-4 py-2 rounded-xl hover:text-white hover:border-slate-700 transition-colors"
                >
                  <ShieldCheck size={16} />
                  Privacy Policy
                </a>
              </div>
            </div>

            {!inApp && (
              <button
                onClick={login}
                className="inline-flex items-center gap-2 bg-primary text-on-primary px-4 py-3 rounded-2xl font-semibold active:scale-95 transition-transform"
              >
                <LogIn size={16} />
                Open the app
              </button>
            )}
            {inApp && (
              <a
                href={appProfile.home_docs_anchor || '/#docs'}
                className="inline-flex items-center gap-2 text-sm text-indigo-300 hover:text-indigo-200"
              >
                <BookOpen size={16} />
                Open the public docs section
              </a>
            )}
            {onOpenEnterprise && (
              <button
                type="button"
                onClick={onOpenEnterprise}
                className="inline-flex items-center gap-2 text-sm text-amber-300 hover:text-amber-200"
              >
                <CalendarDays size={16} />
                Switch to Enterprise
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function LandingContent({ user, login, error, appProfile, scrollToDocs, scrollToEnterprise, copied, setCopied }) {
  if (user) {
    return <HomeDashboard user={user} copied={copied} setCopied={setCopied} />;
  }

  return (
    <div className="space-y-20">
      <div className="relative overflow-hidden border border-slate-800 bg-[radial-gradient(circle_at_top,_rgba(99,102,241,0.18),_transparent_45%),linear-gradient(180deg,_rgba(15,23,42,0.94),_rgba(2,6,23,1))] rounded-[40px] p-8">
        <div className="absolute inset-0 bg-[linear-gradient(120deg,transparent_0%,rgba(59,130,246,0.06)_32%,transparent_70%)]" />
        <div className="relative">
          <div className="grid gap-10 lg:grid-cols-[1.2fr_0.8fr] lg:items-center">
            <div className="space-y-8">
              <div className="space-y-4">
                <p className="text-[10px] uppercase tracking-[0.28em] text-indigo-400 font-label-caps">The Webhook Control Layer</p>
                <h1 className="text-4xl md:text-5xl font-h1 text-white leading-tight">
                  Stop paying attention to every event.
                </h1>
                <p className="text-base text-slate-300 max-w-xl">
                  AgentHook receives events from any app, filters out noise, classifies what matters, and forwards the right payload to the right tool.
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
                  className="inline-flex items-center justify-center gap-2 bg-primary text-on-primary px-6 py-3 rounded-2xl font-bold active:scale-95 transition-transform"
                >
                  <LogIn size={18} />
                  Get Started
                </button>
                <button
                  onClick={scrollToDocs}
                  className="inline-flex items-center justify-center gap-2 border border-slate-700 bg-slate-950/50 px-6 py-3 rounded-2xl font-semibold text-white hover:bg-slate-900 transition-colors"
                >
                  View docs
                </button>
              </div>
            </div>

            <div className="rounded-[28px] border border-slate-800 bg-slate-950/70 p-5 shadow-2xl shadow-indigo-950/20">
              <div className="flex items-center justify-between mb-4">
                <div>
                  <p className="text-white text-sm font-semibold">Ready to scale?</p>
                  <p className="text-[11px] text-slate-500">Autonomous monitors await.</p>
                </div>
                <span className="rounded-full border border-emerald-500/20 bg-emerald-500/10 px-2 py-1 text-[10px] font-bold text-emerald-300">
                  AUTO-READY
                </span>
              </div>
              <div className="space-y-3">
                 <div className="rounded-2xl border border-indigo-500/20 bg-indigo-500/10 p-4">
                  <p className="text-[10px] uppercase tracking-[0.2em] text-indigo-300">Set up Heartbeat</p>
                  <p className="mt-2 text-sm text-indigo-100">Configure AgentHermes to handle the noise in the background.</p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function HomeDashboard({ user, copied, setCopied }) {
  const [apiToken, setApiToken] = useState('');
  const [tokenBusy, setTokenBusy] = useState(false);
  const [apiTokensList, setApiTokensList] = useState([]);
  const [loadingTokens, setLoadingTokens] = useState(false);
  const [error, setError] = useState('');

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

  useEffect(() => {
    fetchTokens();
  }, []);

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

  return (
    <div className="space-y-8 max-w-5xl mx-auto">
      <div className="space-y-2">
        <p className="text-[10px] uppercase tracking-[0.22em] text-indigo-400 font-label-caps">Developer Console</p>
        <h1 className="text-3xl font-h1 text-white">Welcome back, {user?.public_alias || 'Developer'}</h1>
        <p className="text-slate-400 text-sm">Get your webhook infrastructure running autonomously.</p>
      </div>

      <div className="grid gap-6 lg:grid-cols-[0.8fr_1.2fr]">
        <div className="space-y-4">
          <div className="flex items-center justify-between px-1">
            <h3 className="text-white font-semibold">1. Create API Token</h3>
          </div>
          <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-4">
            <p className="text-sm text-slate-300">
              Generate an <code className="text-indigo-300">AGENTHOOK_TOKEN</code> to authenticate your heartbeat agent or CLI.
            </p>
            {error && <InlineNotice tone="error">{error}</InlineNotice>}
            <button
              onClick={createToken}
              disabled={tokenBusy}
              className="w-full bg-primary text-on-primary font-bold py-3 rounded-2xl active:scale-95 transition-transform disabled:opacity-50"
            >
              {tokenBusy ? 'CREATING...' : 'CREATE TOKEN'}
            </button>
            {apiToken && (
              <div className="flex flex-col gap-2 bg-emerald-500/10 px-3 py-3 rounded-xl border border-emerald-500/20">
                <span className="text-[10px] text-emerald-400 font-label-caps">New token created (copy now)</span>
                <div className="flex items-center gap-2">
                  <code className="text-indigo-300 font-code-snippet text-xs truncate break-all">{apiToken}</code>
                  <CopyButton value={apiToken} copiedKey={copied} setCopiedKey={setCopied} copyKey="home-api-token" />
                </div>
              </div>
            )}
            {apiTokensList.length > 0 && (
               <p className="text-[10px] text-slate-500 font-label-caps px-1">You have {apiTokensList.length} active token(s).</p>
            )}
          </div>
        </div>

        <div className="space-y-4">
          <div className="flex items-center justify-between px-1">
            <h3 className="text-white font-semibold">2. Setup Heartbeat</h3>
          </div>
          <HeartbeatTab />
        </div>
      </div>
    </div>
  );
}

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

      <LandingSection eyebrow="What AgentHook does" title="A webhook control layer for founders, ops, and teams.">
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
      </LandingSection>

      <LandingSection eyebrow="OpenClaw ROI" title="Reduce OpenClaw cost by sending only meaningful events.">
        <div className="grid gap-6 lg:grid-cols-[1fr_1fr]">
          <div className="rounded-3xl border border-red-500/10 bg-red-500/5 p-6 space-y-4">
            <p className="text-[10px] uppercase tracking-[0.22em] text-red-300">Without AgentHook</p>
            <h3 className="text-white text-xl font-semibold">Every event reaches downstream automation.</h3>
            <ul className="space-y-2 text-sm text-slate-300">
              <li>Heartbeat and health-check events consume attention.</li>
              <li>Routine metrics trigger unnecessary runs.</li>
              <li>Teams pay to discover events are irrelevant.</li>
            </ul>
          </div>
          <div className="rounded-3xl border border-emerald-500/20 bg-emerald-500/10 p-6 space-y-4">
            <p className="text-[10px] uppercase tracking-[0.22em] text-emerald-300">With AgentHook</p>
            <h3 className="text-white text-xl font-semibold">Only classified, useful events move forward.</h3>
            <ul className="space-y-2 text-sm text-slate-300">
              <li>Heartbeats can be dropped with no action.</li>
              <li>Only high-signal events move forward.</li>
              <li>Lower processing cost and cleaner operational queues.</li>
            </ul>
          </div>
        </div>
      </LandingSection>

      <section className="rounded-[32px] border border-indigo-500/20 bg-[linear-gradient(135deg,rgba(79,70,229,0.16),rgba(15,23,42,0.92))] px-6 py-8 md:px-8 md:py-10">
        <div className="max-w-3xl space-y-4 text-center mx-auto">
          <p className="text-[10px] uppercase tracking-[0.24em] text-indigo-200 font-label-caps">Start now</p>
          <h2 className="text-3xl md:text-4xl font-h1 text-white">Create a listener today.</h2>
          <button
            onClick={login}
            className="inline-flex items-center justify-center gap-2 bg-primary text-on-primary px-8 py-4 rounded-2xl font-bold active:scale-95 transition-transform mt-4"
          >
            <LogIn size={18} />
            Get Started
          </button>
        </div>
      </section>
    </div>
  );
}

function App() {
  const { user, setUser, isAuthenticated, loading, error, login, logout } = useAuth();
  const tabParam = new URLSearchParams(window.location.search).get('tab');
  const [activeTab, setActiveTab] = useState(VALID_TABS.has(tabParam) ? tabParam : (user ? 'storyboard' : 'home'));
  const [appProfile, setAppProfile] = useState(DEFAULT_APP_PROFILE);
  const [copied, setCopied] = useState('');
  const [isDrawerOpen, setIsDrawerOpen] = useState(false);
  const [events, setEvents] = useState([]);
  const [listeners, setListeners] = useState([]);
  const [fetching, setFetching] = useState(false);
  const [activeTag, setActiveTag] = useState(null);
  const [reclassifyingEventIDs, setReclassifyingEventIDs] = useState({});

  const publicAlias = user?.public_alias || user?.slug || '[userkey]';
  const effectiveAppProfile = normalizeAppProfile(user?.app_profile || appProfile);
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
    apiRequest('/api/app-profile')
      .then((data) => setAppProfile(normalizeAppProfile(data)))
      .catch((err) => console.error('Failed to fetch app profile', err));
  }, []);

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

  const openEnterprise = () => {
    setActiveTab('enterprise');
  };

  const openDocs = () => {
    setActiveTab('docs');
  };

  const openHeartbeat = () => {
    setActiveTab('heartbeat');
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

  // We no longer early-return MarketingHome. We show the app shell with a Login button if needed.

  return (
    <div className="min-h-screen bg-surface text-on-surface">
      <EnterpriseBanner onClick={openEnterprise} />
      <TopAppBar user={user} onLogout={logout} onMenuClick={() => setIsDrawerOpen(true)} />
      <SideDrawer 
        isOpen={isDrawerOpen} 
        onClose={() => setIsDrawerOpen(false)} 
        activeTab={activeTab} 
        onTabChange={setActiveTab} 
      />

      <main className={`pt-6 px-4 ${activeTab === 'home' ? 'max-w-6xl' : 'max-w-md'} mx-auto pb-12`}>
        <AnimatePresence mode="wait">
          {activeTab === 'home' && (
            <motion.div
              key="home"
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -10 }}
            >
              <LandingContent 
                user={user}
                login={login} 
                error={error} 
                appProfile={effectiveAppProfile} 
                scrollToDocs={openDocs}
                scrollToEnterprise={openEnterprise}
                copied={copied}
                setCopied={setCopied}
              />
            </motion.div>
          )}

          {activeTab === 'heartbeat' && (
            <HeartbeatTab />
          )}

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

                <div className="bg-emerald-500/5 border border-emerald-500/20 rounded-xl p-4 cursor-pointer hover:bg-emerald-500/10 transition-colors" onClick={openHeartbeat}>
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-emerald-400 font-label-caps text-[10px]">AUTONOMOUS OPERATIONS</span>
                    <Activity size={14} className="text-emerald-400" />
                  </div>
                  <h4 className="text-white text-sm font-semibold mb-1">Set up your 5-min Heartbeat</h4>
                  <p className="text-xs text-slate-400">
                    Let AgentHermes autonomously monitor webhooks and email you only when meaningful actions are found.
                  </p>
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
              appProfile={effectiveAppProfile}
              setUser={setUser}
              onRefresh={refreshAll}
              copied={copied}
              setCopied={setCopied}
            />
          )}

          {activeTab === 'api-tokens' && (
            <ApiTokensTab
              key="api-tokens"
              copied={copied}
              setCopied={setCopied}
            />
          )}

          {activeTab === 'docs' && (
            <motion.div
              key="docs"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="space-y-6"
            >
              <h2 className="px-1 text-white">Docs</h2>
              <DocsContent appProfile={effectiveAppProfile} inApp onOpenEnterprise={openEnterprise} />
            </motion.div>
          )}

          {activeTab === 'enterprise' && (
            <motion.div
              key="enterprise"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="space-y-6"
            >
              <h2 className="px-1 text-white">Enterprise Plan</h2>
              <EnterpriseSection inApp onPrimaryAction={openEnterprise} />
            </motion.div>
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

          {activeTab === 'integration-secrets' && (
            <motion.div
              key="integration-secrets"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="space-y-4"
            >
              <IntegrationSecretsTab listeners={listeners} />
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

const IntegrationSecretsTab = ({ listeners }) => {
  const defaultSecretForm = () => ({
    secret_key: '',
    purpose: '',
    secret_value: '',
  });

  const [secrets, setSecrets] = useState([]);
  const [loadingSecrets, setLoadingSecrets] = useState(false);
  const [savingSecret, setSavingSecret] = useState(false);
  const [notice, setNotice] = useState('');
  const [editingSecretID, setEditingSecretID] = useState('');
  const [secretForm, setSecretForm] = useState(defaultSecretForm);

  const hasSingleTenant = listeners.some((listener) => listener.deployment_mode === 'single_tenant');

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
    fetchSecrets();
  }, []);

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

  const beginEditSecret = (secret) => {
    setEditingSecretID(secret.id);
    setSecretForm({
      secret_key: secret.secret_key || '',
      purpose: secret.purpose || '',
      secret_value: '',
    });
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

  return (
    <motion.div
      initial={{ opacity: 0, x: -20 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: 20 }}
      className="space-y-4"
    >
      <h2 className="px-1 text-white">Integration Secrets</h2>

      <Panel
        title="Secret Registry"
        subtitle="Store named secret refs for OpenClaw, CRMs, and any downstream integration without exposing raw credentials in target config."
        action={<KeyRound size={18} className="text-primary" />}
      >
        <InlineNotice>
          {hasSingleTenant
            ? 'Single-tenant listeners can auto-resolve conventional env vars when no secret ref is attached. Multitenant listeners should still prefer named secret refs.'
            : 'This deployment is multitenant right now, so integrations should attach named secret refs. Env fallback stays an operator override only.'}
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
        {notice && <InlineNotice tone={notice.toLowerCase().includes('success') || notice.toLowerCase().includes('created') || notice.toLowerCase().includes('updated') ? 'success' : 'info'}>{notice}</InlineNotice>}
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
      </Panel>

      <Panel
        title="Saved Secret Refs"
        subtitle="Reference these keys from integration targets instead of embedding tokens directly in config JSON."
        action={
          <button onClick={fetchSecrets} className="text-slate-400 hover:text-white" title="Refresh secret refs">
            <RefreshCw size={16} className={loadingSecrets ? 'animate-spin' : ''} />
          </button>
        }
      >
        <div className="space-y-3">
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
    </motion.div>
  );
};

const IntegrationsTab = () => {
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
  const [targets, setTargets] = useState([]);
  const [secrets, setSecrets] = useState([]);
  const [loadingTargets, setLoadingTargets] = useState(false);
  const [savingTarget, setSavingTarget] = useState(false);
  const [notice, setNotice] = useState('');
  const [expandedTargetID, setExpandedTargetID] = useState('');
  const [editingTargetID, setEditingTargetID] = useState('');
  const [targetForm, setTargetForm] = useState(defaultTargetForm);

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
    try {
      const data = await apiRequest('/api/integration-secrets');
      setSecrets(Array.isArray(data) ? data : []);
    } catch (err) {
      setNotice(err.message);
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
        title="Create Integration"
        subtitle="Define reusable named targets for OpenClaw, custom forward URLs, or any downstream system your skills can call."
        action={<Cable size={18} className="text-primary" />}
      >
        <InlineNotice>
          Use the separate <span className="text-white font-semibold">Integration Secrets</span> tab to create or rotate named secret refs, then attach them here to keep tokens out of target config.
        </InlineNotice>
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

const ApiTokensTab = ({ copied, setCopied }) => {
  const [apiToken, setApiToken] = useState('');
  const [tokenBusy, setTokenBusy] = useState(false);
  const [apiTokensList, setApiTokensList] = useState([]);
  const [loadingTokens, setLoadingTokens] = useState(false);
  const [error, setError] = useState('');

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

  useEffect(() => {
    fetchTokens();
  }, []);

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
      <h2 className="px-1 text-white">API Tokens</h2>
      <Panel
        title="API Tokens"
        subtitle="Manage and generate tokens for curl, scripts, or direct API testing."
        action={<KeyRound size={18} className="text-primary" />}
      >
        {error && <InlineNotice tone="error">{error}</InlineNotice>}
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
    </motion.div>
  );
};

const UrlsTab = ({ listeners, user, appProfile, setUser, onRefresh, copied, setCopied }) => {
  const [provider, setProvider] = useState('github');
  const [listenerID, setListenerID] = useState('');
  const [deploymentMode, setDeploymentMode] = useState(appProfile?.deployment_mode || 'multitenant');
  const [plainTextAction, setPlainTextAction] = useState('store_mysql');
  const [useLLMFallback, setUseLLMFallback] = useState(true);
  const [listenerSecretMode, setListenerSecretMode] = useState('auto');
  const [listenerSecretValue, setListenerSecretValue] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [secretMap, setSecretMap] = useState({});
  const [secretsHistory, setSecretsHistory] = useState({});
  const [webhookIdentities, setWebhookIdentities] = useState([]);
  const [loadingIdentities, setLoadingIdentities] = useState(false);
  const [identityBusyID, setIdentityBusyID] = useState('');
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

  useEffect(() => {
    setDeploymentMode(appProfile?.deployment_mode || 'multitenant');
  }, [appProfile?.deployment_mode]);


  const fetchWebhookIdentities = async () => {
    setLoadingIdentities(true);
    try {
      const data = await apiRequest('/api/webhook-identities');
      setWebhookIdentities(Array.isArray(data) ? data : []);
    } catch (err) {
      console.error('Failed to fetch webhook identities', err);
    } finally {
      setLoadingIdentities(false);
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
    fetchWebhookIdentities();
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
      await fetchWebhookIdentities();
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
      await fetchWebhookIdentities();
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
      await fetchWebhookIdentities();
    } catch (err) {
      setError(err.message);
    }
  };

  const changeIdentityStatus = async (identityID, nextStatus) => {
    setIdentityBusyID(identityID);
    setError('');
    try {
      const path = nextStatus === 'blocked'
        ? `/api/webhook-identities/${identityID}/block`
        : `/api/webhook-identities/${identityID}/restore`;
      await apiRequest(path, { method: 'POST' });
      await fetchWebhookIdentities();
    } catch (err) {
      setError(err.message);
    } finally {
      setIdentityBusyID('');
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
        subtitle="Provision a new ingress scenario directly from the UI, then bind it to a generated or custom secret. This deployment keeps listener mode env-backed and multitenant."
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
              <div className="w-full bg-slate-900 border border-slate-800 rounded-lg px-3 py-2 text-sm text-white">
                {deploymentMode === 'single_tenant' ? 'Single-tenant (env controlled)' : 'Multitenant (env controlled)'}
              </div>
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

      <Panel
        title="Mailbox / Webhook ID Registry"
        subtitle="This exact-ID registry controls whether a short webhook ID or mailbox address is active, blocked, or reserved for your account."
        action={
          <button
            onClick={() => fetchWebhookIdentities().catch((err) => setError(err.message))}
            className="text-slate-400 hover:text-white"
            title="Refresh identity registry"
          >
            <RefreshCw size={16} className={loadingIdentities ? 'animate-spin' : ''} />
          </button>
        }
      >
        <div className="space-y-3">
          {webhookIdentities.map((identity) => {
            const isBusy = identityBusyID === identity.id;
            return (
              <div key={identity.id} className="rounded-2xl border border-slate-800 bg-slate-950/30 p-4 space-y-2">
                <div className="flex items-start justify-between gap-3">
                  <div>
                    <div className="flex items-center gap-2 flex-wrap">
                      <code className="text-indigo-300 text-xs break-all">{identity.email_address}</code>
                      <span className={`text-[10px] uppercase tracking-[0.18em] px-2 py-1 rounded-full border ${
                        identity.status === 'active'
                          ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-300'
                          : identity.status === 'blocked'
                            ? 'border-amber-500/20 bg-amber-500/10 text-amber-300'
                            : 'border-slate-700 bg-slate-900 text-slate-300'
                      }`}>
                        {identity.status}
                      </span>
                    </div>
                    <p className="text-[11px] text-slate-500 mt-1">
                      Secret: <code className="text-slate-300 break-all">{identity.secret_value}</code>
                    </p>
                    {identity.deleted_at && (
                      <p className="text-[11px] text-slate-500">Reserved for this account since {new Date(identity.deleted_at).toLocaleString()}</p>
                    )}
                  </div>
                  <div className="flex gap-2 shrink-0">
                    {identity.status === 'active' && (
                      <button
                        type="button"
                        disabled={isBusy}
                        onClick={() => changeIdentityStatus(identity.id, 'blocked')}
                        className="rounded-lg border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-[11px] font-semibold text-amber-200 disabled:opacity-50"
                      >
                        {isBusy ? 'BLOCKING...' : 'Block'}
                      </button>
                    )}
                    {identity.status === 'blocked' && (
                      <button
                        type="button"
                        disabled={isBusy}
                        onClick={() => changeIdentityStatus(identity.id, 'active')}
                        className="rounded-lg border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-[11px] font-semibold text-emerald-200 disabled:opacity-50"
                      >
                        {isBusy ? 'RESTORING...' : 'Restore'}
                      </button>
                    )}
                    {identity.status === 'deleted_tombstoned' && (
                      <div className="rounded-lg border border-slate-700 bg-slate-900 px-3 py-2 text-[11px] text-slate-400">
                        Recreate the same secret to restore
                      </div>
                    )}
                  </div>
                </div>
              </div>
            );
          })}
          {!webhookIdentities.length && !loadingIdentities && (
            <p className="text-slate-500 text-center py-6">
              No exact webhook identities tracked yet. Create a listener secret to activate both the short URL and mailbox ID.
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

function HeartbeatTab() {
  const setupCommand = `curl -sSL https://app.agenthook.store/setup_hermes_heartbeat.sh | bash`;
  
  return (
    <motion.div
      key="heartbeat"
      initial={{ opacity: 0, x: -20 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: 20 }}
      className="space-y-6"
    >
      <div className="flex items-center justify-between px-1">
        <h2 className="text-white">Autonomous Heartbeat</h2>
        <span className="rounded-full border border-indigo-500/20 bg-indigo-500/10 px-3 py-1 text-[10px] font-bold text-indigo-300 uppercase tracking-wider">
          Hermes Powered
        </span>
      </div>

      <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-4">
        <div className="space-y-2">
          <p className="text-[10px] uppercase tracking-[0.22em] text-indigo-400 font-label-caps">The Concept</p>
          <p className="text-sm text-slate-300">
            Configure <span className="text-white font-semibold">AgentHermes</span> to act as your autonomous operations agent. 
            It will wake up every 5 minutes, fetch your latest webhooks, analyze them for meaningful signals, and email you a summary 
            only when action is needed.
          </p>
        </div>

        <div className="grid gap-4 md:grid-cols-2">
          <div className="rounded-2xl border border-slate-800 bg-slate-900/40 p-4 space-y-2">
            <h4 className="text-white text-sm font-semibold">Self-Learning</h4>
            <p className="text-xs text-slate-400 leading-relaxed">
              Encountered a new webhook? Hermes uses Codex to generate a deterministic Python processor on the fly, 
              saving it to your local <code className="text-indigo-300">processors/</code> library for future use.
            </p>
          </div>
          <div className="rounded-2xl border border-slate-800 bg-slate-900/40 p-4 space-y-2">
            <h4 className="text-white text-sm font-semibold">Zero Noise</h4>
            <p className="text-xs text-slate-400 leading-relaxed">
              If the store is empty or the events are heartbeats, Hermes remains completely silent. You only get an 
              email when there is a summary worth reading.
            </p>
          </div>
        </div>

        <div className="pt-4 border-t border-slate-800 space-y-4">
          <p className="text-[10px] uppercase tracking-[0.22em] text-slate-400 font-label-caps">One-Click Setup</p>
          <p className="text-sm text-slate-300">
            Run this command in your terminal to download and configure the heartbeat agent automatically.
          </p>
          <div className="flex items-center gap-2 bg-slate-950/50 px-3 py-3 rounded-xl border border-slate-800">
            <code className="text-indigo-300 font-code-snippet text-xs truncate flex-1">{setupCommand}</code>
            <button
              onClick={() => {
                navigator.clipboard.writeText(setupCommand);
              }}
              className="p-2 text-slate-400 hover:text-white transition-colors"
            >
              <Copy size={16} />
            </button>
          </div>
          
          <div className="flex flex-col sm:flex-row gap-3 pt-2">
            <a
              href="/setup_hermes_heartbeat.sh"
              download="setup_hermes_heartbeat.sh"
              className="inline-flex items-center justify-center gap-2 bg-primary text-on-primary px-6 py-3 rounded-2xl font-bold active:scale-95 transition-transform"
            >
              <Save size={18} />
              Download Script (.sh)
            </a>
            <a
              href="https://hermes-agent.nousresearch.com/"
              target="_blank"
              className="inline-flex items-center justify-center gap-2 border border-slate-700 bg-slate-950/50 px-6 py-3 rounded-2xl font-semibold text-white hover:bg-slate-900 transition-colors"
            >
              <ArrowUpRight size={18} />
              About AgentHermes
            </a>
          </div>
        </div>
      </div>
      
      <div className="rounded-3xl border border-slate-800 bg-slate-950/40 p-5 space-y-3">
        <p className="text-[10px] uppercase tracking-[0.22em] text-slate-400 font-label-caps">Prerequisites</p>
        <ul className="space-y-2 text-sm text-slate-300">
          <li className="flex items-start gap-2">
            <span className="mt-1.5 w-1.5 h-1.5 rounded-full bg-indigo-500 shrink-0" />
            <span>Node.js installed (v18+)</span>
          </li>
          <li className="flex items-start gap-2">
            <span className="mt-1.5 w-1.5 h-1.5 rounded-full bg-indigo-500 shrink-0" />
            <span>AgentHermes installed: <code className="text-indigo-300">npm install -g @nousresearch/hermes-agent</code></span>
          </li>
          <li className="flex items-start gap-2">
            <span className="mt-1.5 w-1.5 h-1.5 rounded-full bg-indigo-500 shrink-0" />
            <span>Your AGENTHOOK_TOKEN and AGENTMAIL_API_KEY ready</span>
          </li>
        </ul>
      </div>
    </motion.div>
  );
}

export default App;
