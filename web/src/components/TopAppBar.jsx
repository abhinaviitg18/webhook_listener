import React from 'react';
import { Menu, Zap } from 'lucide-react';

export const TopAppBar = () => {
  return (
    <header className="fixed top-0 left-0 w-full z-50 flex items-center justify-between px-4 h-16 bg-slate-950/80 backdrop-blur-lg border-b border-slate-800">
      <div className="flex items-center gap-3">
        <button className="p-2 text-slate-400 hover:bg-slate-800/50 transition-colors active:scale-95 duration-200 rounded-lg">
          <Menu size={20} />
        </button>
        <span className="text-lg font-bold tracking-tight text-white font-h1">HookWeb</span>
      </div>
      <div className="flex items-center gap-4">
        <Zap className="text-primary fill-primary" size={20} />
        <div className="w-8 h-8 rounded-full border border-slate-700 overflow-hidden">
          <img 
            alt="User Profile" 
            className="w-full h-full object-cover" 
            src="https://lh3.googleusercontent.com/aida-public/AB6AXuD276aLI1NbBR4KyTuqgZrkG6sllO8BC4RXKemo6vbCSEjQXlfgREzMyfO477SlTdd1ZKSdPpUgllVuhvbd9yXuP3_351dsx3prsUhZ_ypYoHEWNVVF7sii5gAXyAXuuGhZcK07XGZN2zJrZNTY7Xm1NVSeu1FpBdRH28CYOTq4iwyEKpzIkv4amL7RlCRDRKOOQkKsY-zLno6aU753F2UznZF8U4BIK7KPHbEL9M26l6v8DdevELBZwWbvFlI2FrR_6Zg8uEXKNTxP"
          />
        </div>
      </div>
    </header>
  );
};
