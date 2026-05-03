import React from 'react';
import { LogOut, Menu, Zap } from 'lucide-react';

export const TopAppBar = ({ user, onLogout, onMenuClick }) => {
  return (
    <header className="sticky top-0 w-full z-50 flex items-center justify-between px-4 h-16 bg-slate-950/80 backdrop-blur-lg border-b border-slate-800">
      <div className="flex items-center gap-3">
        <button 
          onClick={onMenuClick}
          className="p-2 text-slate-400 hover:bg-slate-800/50 transition-colors active:scale-95 duration-200 rounded-lg"
        >
          <Menu size={20} />
        </button>
        <span className="text-lg font-bold tracking-tight text-white font-h1">AgentHook</span>
      </div>
      <div className="flex items-center gap-4">
        <div className="hidden sm:flex flex-col items-end leading-tight">
          <span className="text-xs font-medium text-slate-200">{user?.owner_email || 'Signed in'}</span>
          <span className="text-[10px] uppercase tracking-[0.18em] text-slate-500">{user?.public_alias || user?.slug || 'dashboard'}</span>
        </div>
        <Zap className="text-primary fill-primary" size={20} />
        <button
          onClick={onLogout}
          className="p-2 text-slate-400 hover:bg-slate-800/50 hover:text-white transition-colors active:scale-95 duration-200 rounded-lg"
          title="Sign out"
          aria-label="Sign out"
        >
          <LogOut size={18} />
        </button>
        <div className="w-8 h-8 rounded-full border border-slate-700 bg-slate-900 text-slate-200 flex items-center justify-center text-xs font-semibold">
          {(user?.owner_email || 'A').slice(0, 1).toUpperCase()}
        </div>
      </div>
    </header>
  );
};
