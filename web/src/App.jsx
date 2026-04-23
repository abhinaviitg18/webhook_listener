import React, { useState } from 'react';
import { TopAppBar } from './components/TopAppBar';
import { BottomNavBar } from './components/BottomNavBar';
import { Metrics } from './components/Metrics';
import { StoryboardCard } from './components/StoryboardCard';
import { Plus, RefreshCw, Copy, Check, Brain } from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';

const MOCK_EVENTS = [
  {
    status: 'ACTIVE',
    time: '10:05:42',
    story: "Alice bought a 'Large Coffee' for $5.00 using Apple Pay.",
    actions: ['STORE_MYSQL', 'FORWARD_TELEGRAM'],
  },
  {
    status: 'SHADOW',
    time: '10:02:15',
    story: "User 'Bob_Dev' pushed a commit to main with 4 file changes.",
    actions: ['CI_TRIGGER_SILENT'],
  },
  {
    status: 'LEARNING',
    time: '09:58:10',
    story: "A new type of request from 'Stripe_Webhooks' detected. Pattern matching in progress...",
    actions: ['LLM_PROCESSING'],
  },
];

function App() {
  const [activeTab, setActiveTab] = useState('storyboard');
  const [copied, setCopied] = useState(false);

  const copyUrl = () => {
    navigator.clipboard.writeText('hook.web/in/a9k2_L0x');
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="min-h-screen pb-24 bg-surface">
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
              <Metrics />

              <section className="mb-8">
                <div className="bg-indigo-500/5 border border-indigo-500/20 rounded-xl p-4">
                  <div className="flex items-center justify-between mb-2">
                    <span className="text-indigo-400 font-label-caps text-[10px]">YOUR INGRESS URL</span>
                    <RefreshCw size={14} className="text-indigo-400 cursor-pointer" />
                  </div>
                  <div className="flex items-center gap-2 bg-slate-950/50 px-3 py-2 rounded-lg border border-slate-800">
                    <code className="text-indigo-300 font-code-snippet text-xs truncate">hook.web/in/a9k2_L0x</code>
                    <button onClick={copyUrl} className="ml-auto">
                      {copied ? <Check size={14} className="text-green-500" /> : <Copy size={14} className="text-slate-500" />}
                    </button>
                  </div>
                </div>
              </section>

              <section className="space-y-4">
                <div className="flex items-center justify-between px-1">
                  <h2 className="text-on-background">Storyboard</h2>
                  <span className="text-slate-500 text-xs font-medium">LIVE • 4m ago</span>
                </div>
                {MOCK_EVENTS.map((event, i) => (
                  <StoryboardCard key={i} event={event} />
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
              <div className="glass-card border border-slate-800 rounded-2xl p-4 space-y-4">
                <h3 className="font-h2 text-sm text-slate-400">LLM PROVIDER (BYOK)</h3>
                <div className="space-y-3">
                  <div className="space-y-1">
                    <label className="text-[10px] text-slate-500 font-label-caps">PROVIDER</label>
                    <select className="w-full bg-slate-900 border border-slate-800 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-1 focus:ring-primary">
                      <option>Groq (Recommended)</option>
                      <option>OpenAI</option>
                      <option>Anthropic</option>
                    </select>
                  </div>
                  <div className="space-y-1">
                    <label className="text-[10px] text-slate-500 font-label-caps">API KEY</label>
                    <input type="password" placeholder="sk-················" className="w-full bg-slate-900 border border-slate-800 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:ring-1 focus:ring-primary" />
                  </div>
                  <button className="w-full bg-primary text-on-primary font-bold py-2 rounded-lg text-sm active:scale-95 transition-transform">
                    SAVE CONFIG
                  </button>
                </div>
              </div>
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
              <h2 className="text-xl text-white">AI Skills coming soon</h2>
              <p className="text-slate-500 text-sm max-w-[240px]">
                Define complex logic using natural language instructions.
              </p>
            </motion.div>
          )}

          {activeTab === 'urls' && (
            <motion.div
              key="urls"
              initial={{ opacity: 0, x: -20 }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: 20 }}
              className="space-y-4"
            >
              <h2 className="px-1 text-white">Webhook URLs</h2>
              <div className="glass-card border border-slate-800 rounded-2xl p-4 flex items-center justify-between">
                <div>
                  <p className="text-white text-sm font-medium">stripe-invoice</p>
                  <p className="text-slate-500 text-xs">hook.web/in/a9k2_L0x/stripe-invoice</p>
                </div>
                <StatusBadge status="ACTIVE" />
              </div>
              <div className="glass-card border border-slate-800 rounded-2xl p-4 flex items-center justify-between">
                <div>
                  <p className="text-white text-sm font-medium">github-push</p>
                  <p className="text-slate-500 text-xs">hook.web/in/a9k2_L0x/github-push</p>
                </div>
                <StatusBadge status="SHADOW" />
              </div>
            </motion.div>
          )}
        </AnimatePresence>
      </main>

      <button className="fixed right-6 bottom-24 w-14 h-14 bg-primary-container text-on-primary-container rounded-full shadow-2xl flex items-center justify-center active:scale-90 transition-transform duration-150 z-40 border border-white/10">
        <Plus size={32} />
      </button>

      <BottomNavBar activeTab={activeTab} onTabChange={setActiveTab} />
    </div>
  );
}

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
