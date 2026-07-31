import { useState, useEffect, useRef, useCallback } from 'react';
import { Sidebar } from './components/Sidebar';
import { Header } from './components/Header';
import { Dashboard } from './components/Dashboard';
import { CalendarPage, ReportsPage, TradesPage, NewTradePage } from './components/Pages';
import { SettingsPage } from './components/SettingsPage';
import { LoginScreen } from './components/LoginScreen';
import { EntitlementBanner } from './components/EntitlementBanner';
import { AppProvider } from './i18n';
import { Toaster } from 'react-hot-toast';
import { AutoSyncTick, IsCloudConnected, GetEntitlement, SetSyncTarget } from '../wailsjs/go/main/App';
import { EventsOn } from '../wailsjs/runtime/runtime';
import type { backend } from '../wailsjs/go/models';

function AppContent() {
    const [period, setPeriod] = useState('30');
    const [activePage, setActivePage] = useState('dashboard');
    const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

    // null = belum diperiksa. Membedakan "sedang memeriksa" dari "belum login"
    // mencegah layar login berkedip sesaat pada user yang sebenarnya sudah masuk.
    const [isLoggedIn, setIsLoggedIn] = useState<boolean | null>(null);
    const [entitlement, setEntitlement] = useState<backend.EntitlementView | null>(null);

    // Honest account guard: the backend emits 'sync:target' with status 'blocked'
    // when the active MT5 terminal is NOT the account the user chose to sync.
    // We surface it as a persistent banner so a wrong-account state is never hidden.
    const [blocked, setBlocked] = useState<{
        login: string; server: string; targetLogin: string; targetServer: string;
    } | null>(null);

    useEffect(() => {
        const off = EventsOn('sync:target', (e: {
            status?: string; login?: string; server?: string;
            target_login?: string; target_server?: string;
        }) => {
            if (e?.status === 'blocked') {
                setBlocked({
                    login: e.login ?? '', server: e.server ?? '',
                    targetLogin: e.target_login ?? '', targetServer: e.target_server ?? '',
                });
            } else if (e?.status === 'adopted') {
                setBlocked(null); // target now matches the active account
            }
        });
        return () => off();
    }, []);

    const switchTargetToActive = async () => {
        if (!blocked) return;
        await SetSyncTarget(blocked.login, blocked.server);
        setBlocked(null);
    };

    const refreshEntitlement = useCallback(async () => {
        try {
            setEntitlement(await GetEntitlement());
        } catch {
            setEntitlement(null);
        }
    }, []);

    useEffect(() => {
        (async () => {
            try {
                const connected = await IsCloudConnected();
                setIsLoggedIn(connected);
                if (connected) await refreshEntitlement();
            } catch {
                setIsLoggedIn(false);
            }
        })();
    }, [refreshEntitlement]);

    // Auto-sync loop (every 5s): a dumb timer that calls the single Go entrypoint.
    // ALL cadence logic lives in Go (AutoSyncTick): it pushes live open positions
    // every tick and only triggers a closed-trade sync when a position actually
    // closed (volume-change detection), so idle closed-sync server load stays at
    // zero. Full-history sync (leaderboard/stats rebuild) remains the manual button.
    //
    // Go also decides whether the push happens at all: an unentitled plan still
    // pulls MT5 into the local journal, it just does not reach the cloud. The
    // timer stays dumb, so entitlement is never enforced in two places.
    const isSyncingRef = useRef(false);
    useEffect(() => {
        if (!isLoggedIn) return;
        const interval = setInterval(async () => {
            // Guard: skip this tick if the previous cycle is still running.
            // Each cycle opens a native MT5 pipe; overlapping cycles would spawn
            // concurrent MT5 clients and thrash.
            if (isSyncingRef.current) return;
            isSyncingRef.current = true;
            try {
                await AutoSyncTick();
            } catch (err) {
                // Ignore errors silently (e.g. MT5 closed)
            } finally {
                isSyncingRef.current = false;
                // The server can revoke mid-session (subscription lapsed); Go caches
                // that answer, so re-read it to keep the banner honest.
                void refreshEntitlement();
            }
        }, 5000);
        return () => clearInterval(interval);
    }, [isLoggedIn, refreshEntitlement]);

    const renderPage = () => {
        switch (activePage) {
            case 'dashboard': return <Dashboard period={period} />;
            case 'calendar': return <CalendarPage period={period} />;
            case 'reports': return <ReportsPage period={period} />;
            case 'trades': return <TradesPage period={period} />;
            case 'new-trade': return <NewTradePage />;
            case 'settings': return <SettingsPage />;
            default: return <Dashboard period={period} />;
        }
    };

    if (isLoggedIn === null) {
        return <div className="h-screen w-screen bg-dark" />;
    }

    if (!isLoggedIn) {
        return (
            <LoginScreen
                onSuccess={async () => {
                    setIsLoggedIn(true);
                    await refreshEntitlement();
                }}
            />
        );
    }

    return (
        <div className="flex h-screen bg-dark text-gray-200 overflow-hidden font-sans transition-colors duration-300 relative">
            {/* Sidebar */}
            <Sidebar
                activePage={activePage}
                onNavigate={(page) => {
                    setActivePage(page);
                    setIsMobileMenuOpen(false); // Close menu on mobile after nav
                }}
                isOpen={isMobileMenuOpen}
                setIsOpen={setIsMobileMenuOpen}
            />

            {/* Main Content */}
            <div className="flex-1 flex flex-col min-w-0">
                <Header
                    onPeriodChange={setPeriod}
                    activePage={activePage}
                    onMenuClick={() => setIsMobileMenuOpen(true)}
                    entitlement={entitlement}
                    onLogout={() => {
                        setIsLoggedIn(false);
                        setEntitlement(null);
                    }}
                />

                <EntitlementBanner entitlement={entitlement} />

                {blocked && (
                    <div className="m-4 rounded-lg border border-amber-400/40 bg-amber-50 dark:bg-amber-900/20 p-3 text-sm text-amber-900 dark:text-amber-200">
                        <p className="font-semibold">Sync dijeda: akun MT5 aktif bukan target sync kamu.</p>
                        <p className="mt-1">
                            Terminal aktif: <b>akun {blocked.login}</b> ({blocked.server}).
                            Target sync kamu: <b>akun {blocked.targetLogin}</b> ({blocked.targetServer}).
                        </p>
                        <div className="mt-2 flex gap-2">
                            <button
                                onClick={switchTargetToActive}
                                className="px-3 py-1 rounded bg-amber-500 text-white text-xs font-semibold hover:bg-amber-600"
                            >
                                Sync akun {blocked.login} ini
                            </button>
                            <button
                                onClick={() => setBlocked(null)}
                                className="px-3 py-1 rounded border border-amber-400 text-xs font-semibold"
                            >
                                Tetap di akun {blocked.targetLogin}
                            </button>
                        </div>
                    </div>
                )}

                <main className="flex-1 overflow-hidden">
                    {renderPage()}
                </main>
            </div>
        </div>
    );
}

function App() {
    return (
        <AppProvider>
            <Toaster
                position="top-right"
                toastOptions={{
                    style: { background: 'var(--bg-card)', color: 'var(--color-gray-200, #f5f2ee)', border: '1px solid var(--color-gray-800, #241c14)' }
                }}
            />
            <AppContent />
        </AppProvider>
    );
}

export default App;
