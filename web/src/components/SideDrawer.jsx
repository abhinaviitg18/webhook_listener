import React from 'react';
import { Activity, BookOpen, BrainCircuit, Cable, Cpu, Key, KeyRound, Link as LinkIcon, ShieldCheck, X } from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';

export const SideDrawer = ({ activeTab, onTabChange, isOpen, onClose }) => {
    const tabs = [
        { id: 'storyboard', label: 'Storyboard', icon: Activity },
        { id: 'skills', label: 'Skills', icon: BrainCircuit },
        { id: 'integrations', label: 'Integrations', icon: Cable },
        { id: 'integration-secrets', label: 'Secrets', icon: KeyRound },
        { id: 'urls', label: 'URLs', icon: LinkIcon },
        { id: 'api-tokens', label: 'API Tokens', icon: Key },
        { id: 'enterprise', label: 'Enterprise', icon: ShieldCheck },
        { id: 'docs', label: 'Docs', icon: BookOpen },
        { id: 'byok', label: 'BYOK', icon: Cpu },
    ];

    return (
        <AnimatePresence>
            {isOpen && (
                <>
                    {/* Backdrop */}
                    <motion.div
                        initial={{ opacity: 0 }}
                        animate={{ opacity: 1 }}
                        exit={{ opacity: 0 }}
                        onClick={onClose}
                        className="fixed inset-0 bg-slate-950/60 backdrop-blur-sm z-[60]"
                    />

                    {/* Drawer */}
                    <motion.nav
                        initial={{ x: '-100%' }}
                        animate={{ x: 0 }}
                        exit={{ x: '-100%' }}
                        transition={{ type: 'spring', damping: 25, stiffness: 200 }}
                        className="fixed top-0 left-0 bottom-0 w-72 bg-slate-900 border-r border-slate-800 z-[70] flex flex-col shadow-2xl"
                    >
                        {/* Drawer Header */}
                        <div className="p-6 flex items-center justify-between border-b border-slate-800">
                            <span className="text-xl font-bold text-white font-h1">Navigation</span>
                            <button 
                                onClick={onClose}
                                className="p-2 text-slate-400 hover:text-white hover:bg-slate-800 rounded-lg transition-colors"
                            >
                                <X size={20} />
                            </button>
                        </div>

                        {/* Tabs List */}
                        <div className="flex-1 overflow-y-auto p-4 space-y-2">
                            {tabs.map((tab) => {
                                const Icon = tab.icon;
                                const isActive = activeTab === tab.id;

                                return (
                                    <button
                                        key={tab.id}
                                        onClick={() => {
                                            onTabChange(tab.id);
                                            onClose();
                                        }}
                                        className={`w-full flex items-center gap-4 px-4 py-3 rounded-xl transition-all duration-200 ${
                                            isActive 
                                            ? 'bg-indigo-500/10 text-indigo-400 border border-indigo-500/20 shadow-lg shadow-indigo-500/5' 
                                            : 'text-slate-400 hover:text-indigo-300 hover:bg-slate-800/50 border border-transparent'
                                        }`}
                                    >
                                        <Icon size={22} className={isActive ? 'text-indigo-400' : 'text-slate-500'} />
                                        <span className="font-medium text-sm tracking-wide">{tab.label}</span>
                                    </button>
                                );
                            })}
                        </div>

                        {/* Drawer Footer */}
                        <div className="p-6 border-t border-slate-800">
                            <p className="text-[10px] uppercase tracking-widest text-slate-500 font-label-caps text-center">
                                AgentHook v1.0
                            </p>
                        </div>
                    </motion.nav>
                </>
            )}
        </AnimatePresence>
    );
};
