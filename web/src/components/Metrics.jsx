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
        <section className="grid grid-cols-2 gap-3 mb-6">
            <div className="bg-surface-container-low p-4 rounded-xl border border-slate-800/50">
                <p className="text-slate-500 font-label-caps text-[10px] mb-1">ACTIVE HOOKS</p>
                <div className="flex items-end gap-2">
                    <span className="font-h2 text-2xl text-primary">{stats.active}</span>
                    <span className="text-secondary text-[12px] font-medium mb-1">+0</span>
                </div>
            </div>
            <div className="bg-surface-container-low p-4 rounded-xl border border-slate-800/50">
                <p className="text-slate-500 font-label-caps text-[10px] mb-1">ACCURACY %</p>
                <div className="flex items-end gap-2">
                    <span className="font-h2 text-2xl text-on-background">{stats.accuracy}</span>
                    <span className="text-secondary text-[12px] font-medium mb-1">↑</span>
                </div>
            </div>
        </section>
    );
};
