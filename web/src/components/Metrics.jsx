import React from 'react';

export const Metrics = () => {
    return (
        <section className="grid grid-cols-2 gap-3 mb-6">
            <div className="bg-surface-container-low p-4 rounded-xl border border-slate-800/50">
                <p className="text-slate-500 font-label-caps text-[10px] mb-1">ACTIVE HOOKS</p>
                <div className="flex items-end gap-2">
                    <span className="font-h2 text-2xl text-primary">24</span>
                    <span className="text-secondary text-[12px] font-medium mb-1">+2</span>
                </div>
            </div>
            <div className="bg-surface-container-low p-4 rounded-xl border border-slate-800/50">
                <p className="text-slate-500 font-label-caps text-[10px] mb-1">ACCURACY %</p>
                <div className="flex items-end gap-2">
                    <span className="font-h2 text-2xl text-on-background">99.2</span>
                    <span className="text-secondary text-[12px] font-medium mb-1">↑</span>
                </div>
            </div>
        </section>
    );
};
