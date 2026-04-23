import React from 'react';
import { Activity, BrainCircuit, Link as LinkIcon, Settings } from 'lucide-react';
import { motion } from 'framer-motion';

export const BottomNavBar = ({ activeTab, onTabChange }) => {
    const tabs = [
        { id: 'storyboard', label: 'Storyboard', icon: Activity },
        { id: 'skills', label: 'Skills', icon: BrainCircuit },
        { id: 'urls', label: 'URLs', icon: LinkIcon },
        { id: 'settings', label: 'Settings', icon: Settings },
    ];

    return (
        <nav className="fixed bottom-0 left-0 w-full z-50 flex justify-around items-center px-2 py-3 bg-slate-950/90 backdrop-blur-md border-t border-slate-800 shadow-2xl">
            {tabs.map((tab) => {
                const Icon = tab.icon;
                const isActive = activeTab === tab.id;

                return (
                    <button
                        key={tab.id}
                        onClick={() => onTabChange(tab.id)}
                        className={`flex flex-col items-center justify-center px-3 py-1 transition-all active:scale-90 duration-150 rounded-2xl ${isActive ? 'text-indigo-400 bg-indigo-500/10' : 'text-slate-500 hover:text-indigo-300'
                            }`}
                    >
                        <Icon size={20} />
                        <span className="font-h1 text-[10px] font-medium tracking-wide mt-1">{tab.label}</span>
                    </button>
                );
            })}
        </nav>
    );
};
