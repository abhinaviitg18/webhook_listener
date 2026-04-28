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
} from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';
import { useAuth } from './context/AuthContext';

const VALID_TABS = new Set(['storyboard', 'skills', 'urls', 'settings']);

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

const FORCED_ACTION_OPTIONS = ['store_mysql', 'forward_http', 'log_only'];

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

function listenerIngressTemplate(listener, accountSlug) {
  if (!listener) return `https://app.agenthook.store/ingest/${accountSlug}/[provider]/[listener_id]/[secret]`;
  return listener.webhook_url_template || `https://app.agenthook.store/ingest/${accountSlug}/${listener.provider}/${listener.listener_id}/[secret]`;
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

function App() {
  const { user, isAuthenticated, loading, error, login, logout } = useAuth();
  const tabParam = new URLSearchParams(window.location.search).get('tab');
  const [activeTab, setActiveTab] = useState(VALID_TABS.has(tabParam) ? tabParam : 'storyboard');
  const [copied, setCopied] = useState('');
  const [events, setEvents] = useState([]);
  const [listeners, setListeners] = useState([]);
  const [fetching, setFetching] = useState(false);
  const [activeTag, setActiveTag] = useState(null);
  const [reclassifyingEventIDs, setReclassifyingEventIDs] = useState({});

  const accountSlug = user?.slug || '[account]';
  const ingressTemplate = listeners.length > 0
    ? listenerIngressTemplate(listeners[0], accountSlug)
    : `https://app.agenthook.store/ingest/${accountSlug}/[provider]/[listener_id]/[secret]`;

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
    return (
      <div className="min-h-screen bg-surface flex flex-col items-center justify-center p-6 text-center space-y-8">
        <div className="space-y-2">
          <h1 className="text-4xl font-h1 text-white grad-text">AgentHook</h1>
          <p className="text-slate-400 max-w-[280px]">Automate your webhook workflows with Webhook Zen.</p>
        </div>
        {error && (
          <div className="bg-red-500/10 border border-red-500/20 text-red-400 px-4 py-2 rounded-lg text-xs font-medium">
            Authentication failed: {error}
          </div>
        )}
        <button
          onClick={login}
          className="flex items-center gap-2 bg-primary text-on-primary px-8 py-4 rounded-2xl font-bold active:scale-95 transition-transform"
        >
          <LogIn size={20} />
          SIGN IN
        </button>
      </div>
    );
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

          {activeTab === 'settings' && (
            <motion.div
              key="settings"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="space-y-6"
            >
              <h2 className="px-1 text-white">Settings</h2>
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
  const [saving, setSaving] = useState(false);
  const [notice, setNotice] = useState('');

  const save = async () => {
    setSaving(true);
    setNotice('');
    try {
      await apiRequest('/v1/byok/providers', {
        method: 'POST',
        body: JSON.stringify({ provider, api_key: apiKey, is_default: true }),
      });
      setNotice('Provider config saved.');
    } catch (err) {
      setNotice(err.message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Panel
      title="LLM Provider (BYOK)"
      subtitle="Bring your own provider credentials for model-driven classification and response flows."
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
        {notice && <InlineNotice tone={notice.includes('saved') ? 'success' : 'error'}>{notice}</InlineNotice>}
        <button
          onClick={save}
          disabled={saving}
          className="w-full bg-primary text-on-primary font-bold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
        >
          {saving ? 'SAVING...' : 'SAVE CONFIG'}
        </button>
      </div>
    </Panel>
  );
};

const UrlsTab = ({ listeners, user, onRefresh, copied, setCopied }) => {
  const [provider, setProvider] = useState('github');
  const [listenerID, setListenerID] = useState('');
  const [deploymentMode, setDeploymentMode] = useState('multitenant');
  const [plainTextAction, setPlainTextAction] = useState('store_mysql');
  const [useLLMFallback, setUseLLMFallback] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');
  const [apiToken, setApiToken] = useState('');
  const [tokenBusy, setTokenBusy] = useState(false);
  const [apiTokensList, setApiTokensList] = useState([]);
  const [loadingTokens, setLoadingTokens] = useState(false);
  const [secretMap, setSecretMap] = useState({});
  const [secretsHistory, setSecretsHistory] = useState({});
  const [loadingSecrets, setLoadingSecrets] = useState(false);

  const accountSlug = user?.slug || '[account]';

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
      setListeners((prev) => prev.filter((l) => l.listener_id !== listener.listener_id || l.provider !== listener.provider));
      setSecretsHistory((prev) => {
        const next = { ...prev };
        delete next[`${listener.provider}:${listener.listener_id}`];
        return next;
      });
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
        }),
      });
      setSecretMap((current) => ({
        ...current,
        [`${created.provider}:${created.listener_id}`]: created,
      }));
      setListenerID('');
      await onRefresh();
    } catch (err) {
      setError(err.message);
    } finally {
      setSubmitting(false);
    }
  };

  const createSecret = async (listener) => {
    const key = `${listener.provider}:${listener.listener_id}`;
    try {
      const created = await apiRequest(`/v1/listeners/${listener.listener_id}/secrets`, {
        method: 'POST',
        body: JSON.stringify({ provider: listener.provider }),
      });
      setSecretMap((current) => ({ ...current, [key]: created }));
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
        title="Create Listener"
        subtitle="Provision a new ingress scenario directly from the UI, including type, mode, and default action."
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
        subtitle="Each listener can mint a fresh secret-backed ingress URL from the UI."
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
            const mintedURL = createdSecret?.webhook_url || latestBackendSecret?.webhook_url || listenerIngressTemplate(listener, accountSlug);

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

                  {createdSecret?.secret_value && (
                    <InlineNotice tone="success">
                      Fresh secret created. Save the URL now: the raw secret is only returned once.
                    </InlineNotice>
                  )}

                  {history.length > 0 && (
                    <div className="space-y-1.5 pt-1">
                      <p className="text-[10px] text-slate-500 font-label-caps px-1">Other Active Secrets</p>
                      {history.map((s) => (
                        <div key={s.id} className="flex items-center justify-between gap-2 bg-slate-900/40 px-3 py-1.5 rounded-lg border border-slate-800/50 text-[10px]">
                          <code className="text-slate-400 truncate">{s.webhook_url}</code>
                          <span className="text-slate-600 shrink-0">{new Date(s.created_at).toLocaleDateString()}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>

                <div className="flex gap-2">
                  <button
                    onClick={() => createSecret(listener)}
                    className="flex-1 bg-slate-900 border border-slate-800 text-white font-semibold py-2 rounded-lg text-sm active:scale-95 transition-transform"
                  >
                    Generate Secret
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
            subtitle="Create a sensible baseline automatically, then fine-tune prompts for specific message patterns."
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
