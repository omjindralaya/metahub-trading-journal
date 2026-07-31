import React, { useState, useEffect, useRef } from 'react';
import { useAppContext } from '../i18n';
import { Menu, Sun, Moon, Cloud, CloudOff, RefreshCw, LogOut } from 'lucide-react';
import toast from 'react-hot-toast';
import { SyncToCloud, LogoutCloud, OpenUpgradePage } from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { extractApiError, apiErrorCode } from '../lib/apiError';
import type { backend } from '../../wailsjs/go/models';

interface HeaderProps {
    onPeriodChange: (period: string) => void;
    activePage: string;
    onMenuClick: () => void;
    entitlement: backend.EntitlementView | null;
    onLogout: () => void;
}

// Satu id dipakai bersama oleh toast kemajuan, sukses, dan gagal, sehingga
// sync yang panjang menggantikan pesannya sendiri alih-alih menumpuk.
const CLOUD_SYNC_TOAST = 'cloud-sync';

export function Header({ onPeriodChange, activePage, onMenuClick, entitlement, onLogout }: HeaderProps) {
    const { lang, setLang, theme, setTheme, t } = useAppContext();
    const [activePeriod, setActivePeriod] = useState('30');
    const [customStart, setCustomStart] = useState('');
    const [customEnd, setCustomEnd] = useState('');
    const [isSyncing, setIsSyncing] = useState(false);

    // Sync otomatis di latar belakang juga melaporkan kemajuan. Menampilkannya
    // akan memunculkan toast tiap kali sebuah posisi ditutup — bising dan tidak
    // diminta. Kemajuan hanya ditampilkan untuk sync yang user picu sendiri.
    const manualSyncRef = useRef(false);

    useEffect(() => {
        const off = EventsOn('sync:progress', (p: { message?: string }) => {
            if (!manualSyncRef.current || !p?.message) return;
            toast.loading(p.message, { id: CLOUD_SYNC_TOAST });
        });
        return () => off();
    }, []);

    // Login is mandatory to reach this screen, so "connected" is a given. What is
    // NOT a given is whether the plan allows pushing to the cloud.
    const canSync = entitlement?.can_sync_now ?? false;

    const handleSync = async () => {
        if (isSyncing) return; // Prevent concurrent execution from rapid double-clicks

        // Refuse out loud rather than firing a request the server will reject.
        // A button that silently does nothing reads as a bug, not as a paywall.
        if (!canSync) {
            toast.error('Sync ke cloud tidak termasuk dalam paket Anda.');
            void OpenUpgradePage();
            return;
        }

        setIsSyncing(true);
        manualSyncRef.current = true;
        try {
            const result = await SyncToCloud();
            // id yang sama dengan toast kemajuan, supaya ia tergantikan
            // alih-alih menumpuk di layar.
            toast.success(result, { id: CLOUD_SYNC_TOAST });
        } catch (err) {
            const errMsg = extractApiError(err) || "Failed to sync to cloud";
            toast.error(errMsg, { id: CLOUD_SYNC_TOAST });
            if (apiErrorCode(err) === "UNAUTHORIZED" || errMsg.includes("session expired")) {
                onLogout();
            }
        } finally {
            setIsSyncing(false);
            manualSyncRef.current = false;
        }
    };

    const periods = [
        { id: 'today', label: t('today') },
        { id: '30', label: `30 ${t('days')}` },
        { id: '60', label: `60 ${t('days')}` },
        { id: '90', label: `90 ${t('days')}` },
        { id: 'custom', label: t('custom') },
    ];

    const handlePeriodClick = (id: string) => {
        setActivePeriod(id);
        if (id !== 'custom') {
            onPeriodChange(id);
        }
    };

    const applyCustomPeriod = () => {
        if (customStart && customEnd) {
            onPeriodChange(`custom:${customStart}:${customEnd}`);
        } else {
            toast.error('Please select both start and end dates.');
        }
    };

    const getPageTitle = () => {
        switch (activePage) {
            case 'dashboard': return t('dashboard');
            case 'calendar': return t('calendar');
            case 'reports': return t('reports');
            case 'trades': return t('trades');
            case 'journal': return t('journal');
            case 'notebook': return t('notebook');
            case 'new-trade': return t('newTrade');
            default: return 'Trading Journal';
        }
    };

    return (
        <header className="h-16 flex items-center justify-between px-4 md:px-8 bg-dark border-b border-gray-800 transition-colors duration-300 shrink-0">
            {/* Left side (Menu & Title) */}
            <div className="flex items-center gap-3 shrink-0 mr-4">
                <button 
                    onClick={onMenuClick}
                    className="md:hidden text-gray-400 hover:text-white transition-colors"
                >
                    <Menu size={24} />
                </button>
                <h1 className="text-xl lg:text-2xl font-semibold text-white tracking-tight hidden lg:block whitespace-nowrap">{getPageTitle()}</h1>
            </div>

            {/* Right side: period selector shrinks/scrolls; the control cluster stays pinned. */}
            <div className="flex items-center gap-2 md:gap-4 flex-1 min-w-0 justify-end h-full">
                {activePage === 'dashboard' && (
                    <div className="flex items-center gap-2 md:gap-4 min-w-0 overflow-x-auto scrollbar-hide">
                        {/* Custom Date Inputs */}
                        {activePeriod === 'custom' && (
                            <div className="flex items-center gap-2 bg-darkCard border border-gray-800 p-1 rounded-md transition-colors duration-300 shrink-0">
                                <input 
                                    type="date" 
                                    value={customStart}
                                    onChange={(e) => setCustomStart(e.target.value)}
                                    className="bg-dark text-xs md:text-sm text-gray-300 rounded px-1 md:px-2 py-1 outline-none border border-gray-700 transition-colors duration-300" 
                                />
                                <span className="text-gray-500">-</span>
                                <input 
                                    type="date" 
                                    value={customEnd}
                                    onChange={(e) => setCustomEnd(e.target.value)}
                                    className="bg-dark text-xs md:text-sm text-gray-300 rounded px-1 md:px-2 py-1 outline-none border border-gray-700 transition-colors duration-300" 
                                />
                                <button 
                                    onClick={applyCustomPeriod}
                                    className="ml-1 bg-brand text-dark px-2 md:px-3 py-1 text-xs md:text-sm font-medium rounded hover:brightness-110 transition-all shrink-0"
                                >
                                    {t('apply')}
                                </button>
                            </div>
                        )}

                        {/* Period Tabs */}
                        <div className="flex items-center gap-1 md:gap-2 bg-darkCard p-1 rounded-lg border border-gray-800 transition-colors duration-300 shrink-0">
                            {periods.map(p => (
                                <button
                                    key={p.id}
                                    onClick={() => handlePeriodClick(p.id)}
                                    className={`px-2 md:px-4 py-1 md:py-1.5 text-xs md:text-sm font-medium rounded-md transition-all whitespace-nowrap shrink-0 ${
                                        activePeriod === p.id 
                                            ? 'bg-dark text-white shadow-sm border border-gray-700' 
                                            : 'text-gray-400 hover:text-gray-200 hover:bg-gray-800/50'
                                    }`}
                                >
                                    {p.label}
                                </button>
                            ))}
                        </div>
                    </div>
                )}

                {/* Theme & Language Toggles */}
                <div className="flex items-center gap-2 md:gap-3 ml-2 md:ml-4 border-l border-gray-800 pl-2 md:pl-6 h-8 shrink-0">
                    <div className="bg-darkCard rounded-md p-0.5 md:p-1 flex border border-gray-800 transition-colors duration-300">
                        <button 
                            onClick={() => setLang('en')} 
                            className={`px-2 md:px-3 py-1 text-[10px] md:text-xs rounded transition-colors ${lang === 'en' ? 'bg-brand text-dark font-bold shadow-sm' : 'text-gray-400 hover:text-white'}`}
                        >
                            EN
                        </button>
                        <button 
                            onClick={() => setLang('id')} 
                            className={`px-2 md:px-3 py-1 text-[10px] md:text-xs rounded transition-colors ${lang === 'id' ? 'bg-brand text-dark font-bold shadow-sm' : 'text-gray-400 hover:text-white'}`}
                        >
                            ID
                        </button>
                    </div>

                    <button 
                        onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
                        className="w-7 h-7 md:w-8 md:h-8 rounded-full bg-darkCard border border-gray-800 flex items-center justify-center text-gray-400 hover:text-white hover:border-brand transition-all duration-300 shrink-0"
                        title="Toggle Theme"
                    >
                        {theme === 'dark' ? <Sun size={14} className="md:w-4 md:h-4" /> : <Moon size={14} className="md:w-4 md:h-4" />}
                    </button>

                    <button
                        onClick={handleSync}
                        disabled={isSyncing}
                        className={`w-7 h-7 md:w-8 md:h-8 rounded-full border flex items-center justify-center transition-all duration-300 shrink-0 ${
                            canSync
                                ? 'bg-brand/10 border-brand text-brand hover:bg-brand hover:text-dark'
                                : 'bg-darkCard border-gray-800 text-gray-500 hover:text-white hover:border-brand'
                        }`}
                        title={canSync ? 'Sync ke Cloud' : 'Sync ke cloud tidak termasuk dalam paket Anda'}
                    >
                        {isSyncing ? (
                            <RefreshCw size={14} className="md:w-4 md:h-4 animate-spin" />
                        ) : canSync ? (
                            <Cloud size={14} className="md:w-4 md:h-4" />
                        ) : (
                            <CloudOff size={14} className="md:w-4 md:h-4" />
                        )}
                    </button>

                    <button
                        onClick={async () => {
                            await LogoutCloud();
                            onLogout();
                            toast.success('Berhasil keluar');
                        }}
                        className="w-7 h-7 md:w-8 md:h-8 rounded-full border border-brandRed/30 text-brandRed flex items-center justify-center hover:bg-brandRed hover:text-white transition-all duration-300 shrink-0 ml-1"
                        title="Keluar"
                    >
                        <LogOut size={14} className="md:w-4 md:h-4" />
                    </button>
                </div>
            </div>
        </header>
    );
}
