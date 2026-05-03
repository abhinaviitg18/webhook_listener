import React, { useState, useEffect } from 'react';

export const Metrics = ({ isAuthenticated }) => {
    const [stats, setStats] = useState({ active: 0, accuracy: 99.8 });

    useEffect(() => {
        if (!isAuthenticated) return;

        // Fetch listeners for active hooks count
        fetch('/v1/listeners', {
            headers: { 'Accept': 'application/json' },
            credentials: 'include'
        })
            .then(res => res.json())
            .then(data => {
                setStats(prev => ({ ...prev, active: data.length }));
            })
            .catch(err => console.error(err));
    }, [isAuthenticated]);

    return (
        <section className="space-y-3 mb-8">
            <div className="grid grid-cols-2 gap-3">
                <div className="bg-surface-container-low p-4 rounded-xl border border-slate-800/50">
                    <p className="text-slate-500 font-label-caps text-[10px] mb-1">ACTIVE HOOKS</p>
                    <div className="flex items-end gap-2">
                        <span className="font-h2 text-2xl text-primary">{stats.active}</span>
                        <span className="text-slate-500 text-[12px] font-medium mb-1">/ 20</span>
                    </div>
                </div>
                <div className="bg-surface-container-low p-4 rounded-xl border border-slate-800/50">
                    <p className="text-slate-500 font-label-caps text-[10px] mb-1">MONTHLY MESSAGES</p>
                    <div className="flex items-end gap-2">
                        <span className="font-h2 text-2xl text-on-background">0</span>
                        <span className="text-slate-500 text-[12px] font-medium mb-1">/ 3,000</span>
                    </div>
                </div>
            </div>
            <div className="bg-amber-500/5 border border-amber-500/20 rounded-xl p-3 flex items-center gap-3">
                <div className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" />
                <p className="text-[11px] text-amber-200/80">
                    <span className="font-bold text-amber-400">Free Plan:</span> Outbound emails are disabled. Upgrade to Enterprise for full outbound support and custom domain.
                </p>
            </div>
        </section>
    );
};
