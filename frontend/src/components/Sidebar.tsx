import {
    LayoutDashboard,
    Calendar,
    BarChart3,
    List,
    PlusSquare,
    Users,
    RefreshCw,
    Settings,
    X
} from 'lucide-react';
import { FetchFromMT5, GetCloudProfile } from '../../wailsjs/go/main/App';
import { useState, useEffect } from 'react';
import React from 'react';
import { useAppContext } from '../i18n';
import toast from 'react-hot-toast';
import { extractApiError } from '../lib/apiError';
import { Logo } from './Logo';

interface SidebarProps {
    activePage: string;
    onNavigate: (page: string) => void;
    isOpen: boolean;
    setIsOpen: (open: boolean) => void;
}

export function Sidebar({ activePage, onNavigate, isOpen, setIsOpen }: SidebarProps) {
    const { t, searchQuery, setSearchQuery } = useAppContext();
    const [isFetching, setIsFetching] = useState(false);
    const [profile, setProfile] = useState<{username: string, display_name: string, tier: string, is_connected: boolean} | null>(null);

    // Refresh profile every 5 seconds to catch login changes
    useEffect(() => {
        const fetchProfile = async () => {
            try {
                if ((window as any).go && (window as any).go.main && (window as any).go.main.App) {
                    const p = await GetCloudProfile();
                    setProfile(p);
                }
            } catch (e) {
                console.error(e);
            }
        };
        fetchProfile();
        const interval = setInterval(fetchProfile, 5000);
        return () => clearInterval(interval);
    }, []);

    const handleFetch = async () => {
        setIsFetching(true);
        try {
            if ((window as any).go && (window as any).go.main && (window as any).go.main.App) {
                const msg = await FetchFromMT5();
                toast.success(msg);
            } else {
                toast.success("Simulasi Sync MT5 berhasil! (Anda sedang menjalankan aplikasi di dalam Browser)");
            }
        } catch (err) {
            toast.error(extractApiError(err));
        } finally {
            setIsFetching(false);
        }
    };

    return (
        <>
            {/* Mobile overlay */}
            {isOpen && (
                <div 
                    className="fixed inset-0 bg-black/50 z-40 md:hidden"
                    onClick={() => setIsOpen(false)}
                />
            )}

            <aside className={`fixed md:static inset-y-0 left-0 z-50 w-64 h-screen bg-darkCard border-r border-gray-800 flex flex-col justify-between transform transition-transform duration-300 ease-in-out ${isOpen ? 'translate-x-0' : '-translate-x-full md:translate-x-0'}`}>
                <div>
                    {/* Logo Area */}
                    <div className="h-16 flex items-center justify-between px-6 border-b border-gray-800">
                        <div className="flex items-center gap-3">
                            <Logo size={32} />
                            <div className="flex flex-col">
                                <span className="text-lg font-extrabold tracking-tight text-white leading-none mb-0.5">
                                    Meta<span className="text-brand">Hub</span>
                                </span>
                                <span className="text-[9px] text-gray-400 font-medium leading-none">{t('tagline')}</span>
                            </div>
                        </div>
                        <button className="md:hidden text-gray-400 hover:text-white" onClick={() => setIsOpen(false)}>
                            <X size={24} />
                        </button>
                    </div>

                    {/* Search */}
                    <div className="p-4">
                        <input 
                            type="text" 
                            placeholder="Search symbols..."
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            className="w-full bg-dark text-gray-200 placeholder-gray-400 text-sm rounded-md px-4 py-2 border border-gray-700 focus:outline-none focus:border-brand transition-colors duration-300"
                        />
                    </div>

                    {/* Navigation Menu */}
                    <nav className="px-4 space-y-1">
                        <NavItem icon={<LayoutDashboard size={18} />} label={t('dashboard')} id="dashboard" active={activePage === 'dashboard'} onClick={() => onNavigate('dashboard')} />
                        <NavItem icon={<Calendar size={18} />} label={t('calendar')} id="calendar" active={activePage === 'calendar'} onClick={() => onNavigate('calendar')} />
                        <NavItem icon={<BarChart3 size={18} />} label={t('reports')} id="reports" active={activePage === 'reports'} onClick={() => onNavigate('reports')} />
                        <NavItem icon={<List size={18} />} label={t('trades')} id="trades" active={activePage === 'trades'} onClick={() => onNavigate('trades')} />
                        <NavItem icon={<PlusSquare size={18} />} label={t('newTrade')} id="new-trade" active={activePage === 'new-trade'} onClick={() => onNavigate('new-trade')} />
                        <NavItem icon={<Settings size={18} />} label={t('settings')} id="settings" active={activePage === 'settings'} onClick={() => onNavigate('settings')} />
                    </nav>
                </div>

                {/* Bottom Actions & Profile */}
                <div className="p-4 border-t border-gray-800">
                    <button 
                        onClick={handleFetch}
                        disabled={isFetching}
                        className="w-full mb-4 bg-brand hover:brightness-110 text-dark font-semibold py-2 px-4 rounded-md flex items-center justify-center transition-all disabled:opacity-50"
                    >
                        <RefreshCw size={16} className={`mr-2 ${isFetching ? 'animate-spin' : ''}`} />
                        {isFetching ? 'Syncing...' : t('syncMT5')}
                    </button>
                    
                    <div className="flex items-center gap-3">
                        <div className="w-10 h-10 rounded-full bg-gray-700 flex items-center justify-center overflow-hidden">
                            <img 
                                src={profile?.is_connected ? `https://ui-avatars.com/api/?name=${encodeURIComponent(profile.display_name || profile.username)}&background=random` : `https://ui-avatars.com/api/?name=Local+Trader&background=374151&color=9CA3AF`} 
                                alt="Profile" 
                            />
                        </div>
                        <div>
                            <p className="text-sm font-medium text-white truncate max-w-[120px]">
                                {profile?.is_connected ? (profile.display_name || profile.username) : "Local Trader"}
                            </p>
                            <p className="text-xs text-gray-400 capitalize">
                                {profile?.is_connected ? (profile.tier || "Standard Plan") : "Guest Mode"}
                            </p>
                        </div>
                    </div>
                </div>
            </aside>
        </>
    );
}

function NavItem({ icon, label, id, active = false, onClick }: { icon: React.ReactNode, label: string, id: string, active?: boolean, onClick: () => void }) {
    return (
        <button 
            onClick={onClick}
            className={`w-full flex items-center gap-3 px-3 py-2 rounded-md transition-colors ${
                active
                    ? 'bg-brand/10 text-brand font-semibold'
                    : 'text-gray-400 hover:text-white hover:bg-gray-800/50'
            }`}
        >
            {icon}
            <span>{label}</span>
        </button>
    );
}
