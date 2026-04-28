import React, { useState } from 'react';
import { Database, Send, Terminal, Brain, ChevronDown, ChevronUp, Tag } from 'lucide-react';
import { motion, AnimatePresence } from 'framer-motion';

function safeJSONParse(text) {
    try { return text ? JSON.parse(text) : null; } catch { return null; }
}

function prettyJSON(value) {
    return JSON.stringify(value, null, 2);
}

function isStructurallyEmpty(value) {
    if (value === null || value === undefined) return true;
    if (typeof value === 'string') return value.trim() === '';
    if (Array.isArray(value)) return value.length === 0;
    if (typeof value === 'object') return Object.keys(value).length === 0;
    return false;
}

function formatPayload(raw, options = {}) {
    const { treatStructuralEmptyAsEmpty = false } = options;
    if (!raw || typeof raw !== 'string' || !raw.trim() || raw.trim() === 'null') return '';
    const parsed = safeJSONParse(raw);
    if (parsed !== null) {
        if (treatStructuralEmptyAsEmpty && isStructurallyEmpty(parsed)) return '';
        return prettyJSON(parsed);
    }
    return raw.trim();
}

function truncateLines(text, maxLines = 35) {
    if (!text) return { truncated: '', isTruncated: false };
    const lines = text.split('\n');
    if (lines.length <= maxLines) return { truncated: text, isTruncated: false };
    return { truncated: lines.slice(0, maxLines).join('\n') + '\n...', isTruncated: true };
}

const StatusBadge = ({ status }) => {
    const configs = {
        ACTIVE: { color: 'text-stage-active', bg: 'bg-stage-active/10', border: 'border-stage-active/20' },
        SHADOW: { color: 'text-stage-shadow', bg: 'bg-stage-shadow/10', border: 'border-stage-shadow/20' },
        LEARNING: { color: 'text-stage-learning', bg: 'bg-stage-learning/10', border: 'border-stage-learning/20', pulse: true },
        processed: { color: 'text-stage-active', bg: 'bg-stage-active/10', border: 'border-stage-active/20' },
    };
    const config = configs[status] || configs.ACTIVE;
    return (
        <div className={`${config.bg} ${config.color} text-[10px] font-bold px-2 py-0.5 rounded-full border ${config.border} flex items-center gap-1`}>
            {config.pulse && <span className="w-1.5 h-1.5 bg-stage-learning rounded-full animate-pulse" />}
            {(status || 'ACTIVE').toUpperCase()}
        </div>
    );
};

const ActionIcon = ({ name }) => {
    const icons = {
        STORE_MYSQL: Database,
        store_mysql: Database,
        FORWARD_TELEGRAM: Send,
        forward_http: Send,
        CI_TRIGGER_SILENT: Terminal,
        LLM_PROCESSING: Brain,
        log_only: Terminal,
    };
    const Icon = icons[name] || Database;
    return <Icon size={14} className="text-slate-400" />;
};

const TagPill = ({ tag, onClick, isMarketing }) => {
    const base = isMarketing
        ? 'bg-amber-500/10 text-amber-400 border-amber-500/20'
        : 'bg-indigo-500/10 text-indigo-400 border-indigo-500/20';
    return (
        <button
            onClick={(e) => { e.stopPropagation(); onClick(tag); }}
            className={`inline-flex items-center gap-1 px-2 py-0.5 rounded-full border text-[10px] font-semibold transition-all hover:scale-105 hover:brightness-125 cursor-pointer ${base}`}
        >
            <Tag size={9} />
            {tag}
        </button>
    );
};

export const StoryboardCard = ({ event, onTagClick }) => {
    const [expanded, setExpanded] = useState(false);

    const tags = safeJSONParse(event.tagsJson) || [];
    const isMarketing = tags.some(t => ['marketing', 'promotion', 'newsletter'].includes(t?.toLowerCase?.()));

    const processedText = formatPayload(event.processedText, { treatStructuralEmptyAsEmpty: true });
    const rawPayload = formatPayload(event.rawPayload);
    // Fallback: if no processed text, show payload as the primary content
    const displayText = processedText || rawPayload || `Received ${event.typeKey || 'webhook'} payload.`;
    const displayLabel = processedText ? 'Processed Summary' : 'Payload';
    const hasRawBehind = processedText && rawPayload && rawPayload !== processedText;
    const { truncated, isTruncated } = truncateLines(displayText);

    const borderColors = {
        ACTIVE: 'border-stage-active/20',
        SHADOW: 'border-stage-shadow/20',
        LEARNING: 'border-stage-learning/20',
        processed: 'border-stage-active/20',
    };

    const accentColors = {
        ACTIVE: 'bg-stage-active',
        SHADOW: 'bg-stage-shadow',
        LEARNING: 'bg-stage-learning',
        processed: 'bg-stage-active',
    };

    return (
        <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            whileHover={{ y: -4, boxShadow: '0 8px 30px rgba(0,0,0,0.12)' }}
            className={`glass-card border rounded-2xl p-4 relative overflow-hidden transition-all duration-300 ${borderColors[event.status] || 'border-slate-800'} ${isMarketing ? 'opacity-70' : ''}`}
        >
            <div className={`absolute top-0 left-0 w-1 h-full ${isMarketing ? 'bg-amber-500/60' : (accentColors[event.status] || 'bg-stage-active')}`} />

            {/* Header */}
            <div className="flex justify-between items-start mb-3">
                <div className="flex items-center gap-2">
                    <span className="font-code-snippet text-[11px] text-slate-500">{event.time}</span>
                    {isMarketing && (
                        <span className="text-[9px] font-bold text-amber-500 bg-amber-500/10 border border-amber-500/20 px-1.5 py-0.5 rounded-full">
                            MARKETING
                        </span>
                    )}
                </div>
                <StatusBadge status={event.status} />
            </div>

            {/* Primary content - collapsed */}
            <div className="mb-3 rounded-xl border border-slate-800 bg-slate-950/60 p-3">
                <div className="mb-2 text-[10px] font-label-caps text-slate-500">
                    {displayLabel}
                </div>
                <pre className="whitespace-pre-wrap break-words font-code-snippet text-[11px] leading-relaxed text-indigo-300">
                    {expanded ? displayText : truncated}
                </pre>
                {isTruncated && (
                    <button
                        onClick={() => setExpanded(!expanded)}
                        className="mt-2 flex items-center gap-1 text-[10px] text-primary font-semibold hover:brightness-125 transition-all"
                    >
                        {expanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
                        {expanded ? 'Show less' : `Show full (${displayText.split('\n').length} lines)`}
                    </button>
                )}
                {!isTruncated && hasRawBehind && (
                    <button
                        onClick={() => setExpanded(!expanded)}
                        className="mt-2 flex items-center gap-1 text-[10px] text-primary font-semibold hover:brightness-125 transition-all"
                    >
                        {expanded ? <ChevronUp size={12} /> : <ChevronDown size={12} />}
                        {expanded ? 'Hide raw payload' : 'Show raw payload'}
                    </button>
                )}
            </div>

            {/* Expanded: raw payload (only when there's a distinct processed text) */}
            <AnimatePresence>
                {expanded && hasRawBehind && (
                    <motion.div
                        initial={{ opacity: 0, height: 0 }}
                        animate={{ opacity: 1, height: 'auto' }}
                        exit={{ opacity: 0, height: 0 }}
                        className="mb-3 rounded-xl border border-slate-800/60 bg-slate-950/40 p-3 overflow-hidden"
                    >
                        <div className="mb-2 text-[10px] font-label-caps text-slate-600">
                            Raw Payload
                        </div>
                        <pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words font-code-snippet text-[10px] leading-relaxed text-slate-500">
                            {rawPayload}
                        </pre>
                    </motion.div>
                )}
            </AnimatePresence>

            {/* Tags */}
            {tags.length > 0 && (
                <div className="flex items-center gap-1.5 flex-wrap mb-3">
                    {tags.map((tag, i) => (
                        <TagPill
                            key={i}
                            tag={tag}
                            isMarketing={['marketing', 'promotion', 'newsletter'].includes(tag?.toLowerCase?.())}
                            onClick={onTagClick || (() => { })}
                        />
                    ))}
                </div>
            )}

            {/* Actions */}
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
