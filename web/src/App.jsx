import React, { useState, useEffect } from 'react';
import { TopAppBar } from './components/TopAppBar';
import { BottomNavBar } from './components/BottomNavBar';
import { Metrics } from './components/Metrics';
import { StoryboardCard } from './components/StoryboardCard';
import { Plus, RefreshCw, Copy, Check, Brain, LogIn } from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';
import { useAuth } from './context/AuthContext';

const VALID_TABS = new Set(['storyboard', 'skills', 'urls', 'settings']);

function App() {
  const { user, token, loading, login } = useAuth();
  const tabParam = new URLSearchParams(window.location.search).get('tab');
  const [activeTab, setActiveTab] = useState(VALID_TABS.has(tabParam) ? tabParam : 'storyboard');
  const [copied, setCopied] = useState(false);
  const [events, setEvents] = useState([]);
  const [listeners, setListeners] = useState([]);
  const [fetching, setFetching] = useState(false);

  useEffect(() => {
    if (!token) return;
    fetchListeners();
    if (activeTab === 'storyboard') {
      fetchEvents();
    }
  }, [token, activeTab]);

  const fetchListeners = async () => {
    try {
      const res = await fetch('/v1/listeners', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setListeners(data || []);
      }
    } catch (err) {
      console.error('Failed to fetch listeners', err);
    }
  };

  const ingressTemplate = listeners.length > 0
    ? `agenthook.store/ingest/${user.slug}/${listeners[0].provider}/${listeners[0].listener_id}/[secret]`
    : `agenthook.store/ingest/${user.slug}/[provider]/[listener_id]/[secret]`;

  const fetchEvents = async () => {
    setFetching(true);
    try {
      const res = await fetch('/api/events', {
        headers: { 'Authorization': `Bearer ${token}` }
      });
      if (res.ok) {
        const data = await res.json();
        setEvents(data || []);
      }
    } catch (err) {
      console.error('Failed to fetch events', err);
    } finally {
      setFetching(false);
    }
  };

  const copyUrl = () => {
    const ingress = user ? ingressTemplate : 'agenthook.store/login';
    navigator.clipboard.writeText(ingress);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
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
      <TopAppBar />

      <main className="pt-20 px-4 max-w-md mx-auto">
        <AnimatePresence mode="wait">
          {activeTab === 'storyboard' && (
            <motion.div
              key="storyboard"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
            >
              <Metrics token={token} />

              <section className="mb-8">
                <div className="bg-indigo-500/5 border border-indigo-500/20 rounded-xl p-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-indigo-400 font-label-caps text-[10px]">YOUR INGRESS URL (AUTO)</span>
                    <RefreshCw
                      size={14}
                      className={`text-indigo-400 cursor-pointer ${fetching ? 'animate-spin' : ''}`}
                      onClick={() => {
                        fetchEvents();
                        fetchListeners();
                      }}
                    />
                  </div>
                  <div className="flex items-center gap-2 bg-slate-950/50 px-3 py-2 rounded-lg border border-slate-800">
                    <code className="text-indigo-300 font-code-snippet text-xs truncate">
                      {ingressTemplate}
                    </code>
                    <button onClick={copyUrl} className="ml-auto text-slate-500 hover:text-white">
                      {copied ? <Check size={14} className="text-green-500" /> : <Copy size={14} />}
                    </button>
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

                {events.length === 0 && !fetching && (
                  <div className="py-12 text-center space-y-3">
                    <div className="inline-flex p-4 bg-slate-900 rounded-full border border-slate-800 text-slate-500">
                      <RefreshCw size={32} />
                    </div>
                    <p className="text-slate-400 text-sm">No events detected yet.<br />Send a payload to your ingress URL.</p>
                  </div>
                )}

                {events.map((event, i) => (
                  <StoryboardCard key={event.id || i} event={{
                    status: event.status,
                    time: new Date(event.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }),
                    story: event.processed_text || `Received ${event.type_key || 'webhook'} payload.`,
                    actions: event.action_selected ? [event.action_selected] : ['LOGGED']
                  }} />
                ))}
              </section>
            </motion.div>
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
              <BYOKSettings token={token} />
            </motion.div>
          )}

          {activeTab === 'skills' && (
            <motion.div
              key="skills"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="flex flex-col items-center justify-center py-20 text-center space-y-4"
            >
              <div className="bg-slate-900 p-6 rounded-full border border-slate-800">
                <Brain size={48} className="text-primary" />
              </div>
              <h2 className="text-xl text-white">AI Skills</h2>
              <p className="text-slate-500 text-sm max-w-[240px]">
                Skill policies are available through the API and appear here once configured.
              </p>
            </motion.div>
          )}

          {activeTab === 'urls' && <UrlsTab listeners={listeners} user={user} />}
        </AnimatePresence>
      </main>

      <button className="fixed right-6 bottom-24 w-14 h-14 bg-primary-container text-on-primary-container rounded-full shadow-2xl flex items-center justify-center active:scale-90 transition-transform duration-150 z-40 border border-white/10">
        <Plus size={32} />
      </button>

      <BottomNavBar activeTab={activeTab} onTabChange={setActiveTab} />
    </div>
  );
}

// Sub-components moved for clarity or can stay here
const BYOKSettings = ({ token }) => {
  const [provider, setProvider] = useState('groq');
  const [apiKey, setApiKey] = useState('');
  const [saving, setSaving] = useState(false);

  const save = async () => {
    setSaving(true);
    try {
      await fetch('/v1/byok/providers', {
        method: 'POST',
        headers: {
          'Authorization': `Bearer ${token}`,
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ provider, api_key: apiKey, is_default: true })
      });
      alert('Config saved!');
    } catch (err) {
      console.error(err);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="glass-card border border-slate-800 rounded-2xl p-4 space-y-4">
      <h3 className="font-h2 text-sm text-slate-400">LLM PROVIDER (BYOK)</h3>
      <div className="space-y-3">
        <div className="space-y-1">
          <label className="text-[10px] text-slate-500 font-label-caps">PROVIDER</label>
          <select
            value={provider}
            onChange={(e) => setProvider(e.target.value)}
            className="w-full bg-slate-900 border border-slate-800 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-1 focus:ring-primary"
          >
            <option value="groq">Groq (Recommended)</option>
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic</option>
          </select>
        </div>
        <div className="space-y-1">
          <label className="text-[10px] text-slate-500 font-label-caps">API KEY</label>
          <input
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder="sk-················"
            className="w-full bg-slate-900 border border-slate-800 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-1 focus:ring-primary"
          />
        </div>
        <button
          onClick={save}
          disabled={saving}
          className="w-full bg-primary text-on-primary font-bold py-2 rounded-lg text-sm active:scale-95 transition-transform disabled:opacity-50"
        >
          {saving ? 'SAVING...' : 'SAVE CONFIG'}
        </button>
      </div>
    </div>
  );
};

const UrlsTab = ({ listeners, user }) => {
  const [copiedListener, setCopiedListener] = useState('');

  const copyListenerUrl = (listener) => {
    const value = `agenthook.store/ingest/${user.slug}/${listener.provider}/${listener.listener_id}/[secret]`;
    navigator.clipboard.writeText(value);
    setCopiedListener(listener.listener_id);
    setTimeout(() => setCopiedListener(''), 1200);
  };

  return (
    <motion.div
      key="urls"
      initial={{ opacity: 0, x: -20 }}
      animate={{ opacity: 1, x: 0 }}
      exit={{ opacity: 0, x: 20 }}
      className="space-y-4"
    >
      <h2 className="px-1 text-white">Webhook URLs</h2>
      {listeners.map((l, i) => (
        <div key={i} className="glass-card border border-slate-800 rounded-2xl p-4 flex items-center justify-between gap-3">
          <div className="min-w-0">
            <p className="text-white text-sm font-medium truncate">{l.provider} · {l.listener_id}</p>
            <p className="text-slate-500 text-[10px] break-all">agenthook.store/ingest/{user.slug}/{l.provider}/{l.listener_id}/[secret]</p>
          </div>
          <div className="flex items-center gap-2 shrink-0">
            <button
              onClick={() => copyListenerUrl(l)}
              className="text-slate-500 hover:text-white"
              title="Copy ingress template"
            >
              {copiedListener === l.listener_id ? <Check size={14} className="text-green-500" /> : <Copy size={14} />}
            </button>
            <StatusBadge status="ACTIVE" />
          </div>
        </div>
      ))}
      {listeners.length === 0 && <p className="text-slate-500 text-center py-10">No specific URLs configured yet.</p>}
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
