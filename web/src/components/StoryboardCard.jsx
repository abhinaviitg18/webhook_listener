import React from 'react';
import { Database, Send, Terminal, Brain } from 'lucide-react';
import { motion } from 'framer-motion';

const StatusBadge = ({ status }) => {
    const configs = {
        ACTIVE: { color: 'text-stage-active', bg: 'bg-stage-active/10', border: 'border-stage-active/20' },
        SHADOW: { color: 'text-stage-shadow', bg: 'bg-stage-shadow/10', border: 'border-stage-shadow/20' },
        LEARNING: { color: 'text-stage-learning', bg: 'bg-stage-learning/10', border: 'border-stage-learning/20', pulse: true },
    };

    const config = configs[status] || configs.ACTIVE;

    return (
        <div className={`${config.bg} ${config.color} text-[10px] font-bold px-2 py-0.5 rounded-full border ${config.border} flex items-center gap-1`}>
            {config.pulse && <span className="w-1.5 h-1.5 bg-stage-learning rounded-full animate-pulse" />}
            {status}
        </div>
    );
};

const ActionIcon = ({ name }) => {
    const icons = {
        STORE_MYSQL: Database,
        FORWARD_TELEGRAM: Send,
        CI_TRIGGER_SILENT: Terminal,
        LLM_PROCESSING: Brain,
    };
    const Icon = icons[name] || Database;
    return <Icon size={14} className="text-slate-400" />;
};

export const StoryboardCard = ({ event }) => {
    const borderColors = {
        ACTIVE: 'border-stage-active/20',
        SHADOW: 'border-stage-shadow/20',
        LEARNING: 'border-stage-learning/20',
    };

    const accentColors = {
        ACTIVE: 'bg-stage-active',
        SHADOW: 'bg-stage-shadow',
        LEARNING: 'bg-stage-learning',
    };

    return (
        <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            whileHover={{ y: -4, boxShadow: '0 8px 30px rgba(0,0,0,0.12)' }}
            className={`glass-card border rounded-2xl p-4 relative overflow-hidden transition-all duration-300 ${borderColors[event.status]}`}
        >
            <div className={`absolute top-0 left-0 w-1 h-full ${accentColors[event.status]}`} />
            <div className="flex justify-between items-start mb-3">
                <span className="font-code-snippet text-[11px] text-slate-500">{event.time}</span>
                <StatusBadge status={event.status} />
            </div>
            <p className={`font-story-text text-text-story text-sm mb-4 leading-relaxed ${event.status === 'LEARNING' ? 'opacity-80 italic' : ''}`}>
                {event.story}
            </p>
            <div className="flex items-center gap-2 flex-wrap">
                {event.actions.map((action, i) => (
                    <motion.div
                        key={i}
                        whileHover={{ scale: 1.05 }}
                        className="flex items-center gap-1.5 bg-slate-900 px-2 py-1 rounded border border-slate-800"
                    >
                        <div className={action === 'LLM_PROCESSING' ? 'animate-pulse text-primary' : ''}>
                            <ActionIcon name={action} />
                        </div>
                        <span className="font-code-snippet text-[10px] text-slate-300">{action}</span>
                    </motion.div>
                ))}
            </div>
        </motion.div>
    );
};
