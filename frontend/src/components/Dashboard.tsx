import { useEffect, useState } from 'react';
import { GetDashboardData, GetCurrency, GetMT5AccountInfo } from '../../wailsjs/go/main/App';
import { useAppContext } from '../i18n';
import { formatMoney } from '../lib/format';
import { 
    LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
    PieChart, Pie, Cell, BarChart, Bar, ReferenceLine, Label 
} from 'recharts';

export function Dashboard({ period }: { period: string }) {
    const { lang, t, searchQuery } = useAppContext();
    const [trades, setTrades] = useState<any[]>([]);
    const [currency, setCurrency] = useState<string>("$");
    const [account, setAccount] = useState<{ login: number; account_type: string; server: string } | null>(null);
    // Bumped whenever the live MT5 account changes, to re-run loadData with the
    // CURRENT period (calling loadData directly from the poll would capture a stale
    // period closure).
    const [reloadTick, setReloadTick] = useState(0);

    useEffect(() => {
        loadData();
    }, [period, reloadTick]);

    // Poll the live MT5 account so the header always reflects the terminal that is
    // ACTUALLY open — not whichever account was open when the page first mounted.
    // When the logged-in account changes under us (user switched accounts in MT5),
    // reload the trade list too, so the header and the data below it can never
    // disagree about which account is on screen.
    useEffect(() => {
        let cancelled = false;
        const poll = async () => {
            try {
                const info = await GetMT5AccountInfo();
                if (cancelled) return;
                setAccount(prev => {
                    if (prev && (prev.login !== info.login || prev.server !== info.server)) {
                        setReloadTick(t => t + 1); // account switched — refresh trades to match the new header
                    }
                    return { login: info.login, account_type: info.account_type, server: info.server };
                });
            } catch {
                if (!cancelled) setAccount(null); // MT5 not open — leave header hidden
            }
        };
        poll();
        const id = setInterval(poll, 5000);
        return () => { cancelled = true; clearInterval(id); };
    }, []);

    const loadData = async () => {
        try {
            if ((window as any).go && (window as any).go.main && (window as any).go.main.App) {
                const data = await GetDashboardData(period);
                setTrades(data || []);
                const curr = await GetCurrency();
                setCurrency(curr || "$");
            } else {
                setCurrency("$");
                setTrades([
                    { net_profit: 150.5, symbol: "XAUUSD", type: "Buy", close_time: new Date(Date.now() - 86400000*2).toISOString() },
                    { net_profit: -45.2, symbol: "EURUSD", type: "Sell", close_time: new Date(Date.now() - 86400000).toISOString() },
                    { net_profit: 320.0, symbol: "BTCUSD", type: "Buy", close_time: new Date().toISOString() }
                ]);
            }
        } catch (err) {
            console.error("Failed to fetch dashboard data:", err);
        }
    };

    const filteredTrades = trades.filter(t => t.symbol.toLowerCase().includes(searchQuery.toLowerCase()));

    const totalPnL = filteredTrades.reduce((acc, t) => acc + t.net_profit, 0);
    const winTrades = filteredTrades.filter(t => t.net_profit > 0);
    const lossTrades = filteredTrades.filter(t => t.net_profit <= 0);
    const winCount = winTrades.length;
    const lossCount = lossTrades.length;
    const avgWin = winCount > 0 ? winTrades.reduce((acc, t) => acc + t.net_profit, 0) / winCount : 0;
    const avgLoss = lossCount > 0 ? Math.abs(lossTrades.reduce((acc, t) => acc + t.net_profit, 0) / lossCount) : 0;

    let cumulative = 0;
    const pnlData = [...filteredTrades].reverse().map(t => {
        cumulative += t.net_profit;
        return { 
            name: new Date(t.close_time).toLocaleString(lang === 'id' ? 'id-ID' : 'en-GB', { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false }), 
            pnl: cumulative 
        };
    });

    const totalTradesCount = winCount + lossCount;
    const winRate = totalTradesCount > 0 ? ((winCount / totalTradesCount) * 100).toFixed(0) + '%' : '0%';
    const donutData = [
        { name: 'Wins', value: winCount },
        { name: 'Losses', value: lossCount },
    ];
    const COLORS = ['var(--profit)', 'var(--loss)'];
    const barData = [{ name: 'Average', win: avgWin, loss: -avgLoss }];

    return (
        <div className="p-4 md:p-8 h-full overflow-y-auto">
            {account && account.login > 0 && (
                <div className="mb-3 text-xs text-gray-500 dark:text-gray-400">
                    {t('showingActiveAccount')}: <b className="text-gray-300">{account.login}</b>
                    <span className="ml-1">({account.account_type} · {account.server})</span>
                </div>
            )}
            {/* Top Stats Row */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 md:gap-6 mb-6">
                <StatCard title={t('totalPnL')} value={formatMoney(currency, totalPnL, lang)} isPositive={totalPnL >= 0} />
                <StatCard title={t('totalTrades')} value={filteredTrades.length} />
                <StatCard title={t('winTrades')} value={winCount} isPositive={true} />
                <StatCard title={t('lossTrades')} value={lossCount} isPositive={false} />
            </div>

            {/* Main Grid Area */}
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-4 md:gap-6 mb-6">
                {/* PnL Line Chart */}
                <div className="lg:col-span-2 bg-darkCard border border-gray-800 rounded-xl p-4 md:p-5 min-h-[300px] transition-colors duration-300">
                    <h3 className="text-sm font-semibold text-gray-300 mb-4 uppercase tracking-wider">{t('cumulativePnL')}</h3>
                    <div className="h-64">
                        <ResponsiveContainer width="100%" height="100%">
                            <LineChart data={pnlData}>
                                <CartesianGrid strokeDasharray="3 3" stroke="#374151" vertical={false} />
                                <XAxis dataKey="name" stroke="#9CA3AF" tick={{fontSize: 10}} />
                                <YAxis stroke="#9CA3AF" tick={{fontSize: 10}} tickFormatter={(value) => `${currency}${value}`} width={50} />
                                <Tooltip 
                                    contentStyle={{ backgroundColor: 'var(--color-darkCard)', borderColor: 'var(--color-gray-800)', color: 'var(--color-gray-200)' }}
                                    formatter={(value: any) => [`${currency}${Number(value).toFixed(2)}`, 'PnL']}
                                />
                                <Line type="monotone" dataKey="pnl" stroke={totalPnL >= 0 ? 'var(--profit)' : 'var(--loss)'} strokeWidth={3} dot={false} />
                            </LineChart>
                        </ResponsiveContainer>
                    </div>
                </div>

                {/* Right Column */}
                <div className="col-span-1 bg-darkCard border border-gray-800 rounded-xl p-4 md:p-5 min-h-[300px] max-h-[400px] lg:max-h-none overflow-y-auto transition-colors duration-300">
                    <h3 className="text-sm font-semibold text-gray-300 mb-4 uppercase tracking-wider">{t('recentTrades')}</h3>
                    <div className="space-y-4">
                        {filteredTrades.length === 0 ? (
                            <div className="text-center text-gray-500 py-4 text-sm">No trades found</div>
                        ) : filteredTrades.slice(0, 10).map((t, i) => (
                            <div key={i} className="flex justify-between items-center gap-2 text-sm border-b border-gray-800 pb-2">
                                <div className="min-w-0">
                                    <span className="text-white font-medium flex items-center">
                                        <span className="truncate">{t.symbol}</span>
                                        {t.account_type && t.account_type !== "Unknown" && (
                                            <span className={`ml-2 px-1.5 py-0.5 rounded text-[9px] uppercase font-bold tracking-wider shrink-0 ${t.account_type === 'Real' ? 'bg-brand/10 text-brand dark:bg-brand/20 dark:text-brand border border-brand/20 dark:border-brand/30' : 'bg-brandGreen/10 text-brandGreen dark:bg-brandGreen/20 dark:text-brandGreen border border-brandGreen/20 dark:border-brandGreen/30'}`}>
                                                {t.account_type}
                                            </span>
                                        )}
                                    </span>
                                    <span className="text-xs text-gray-500">{t.type}</span>
                                </div>
                                <span className={`font-semibold tabular-nums whitespace-nowrap shrink-0 ${t.net_profit >= 0 ? 'text-brandGreen' : 'text-brandRed'}`}>
                                    {formatMoney(currency, t.net_profit, lang)}
                                </span>
                            </div>
                        ))}
                    </div>
                </div>
            </div>

            {/* Bottom Cards Area */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4 md:gap-6">
                 {/* Win / Loss Donut */}
                 <div className="bg-darkCard border border-gray-800 rounded-xl p-4 md:p-5 h-[250px] transition-colors duration-300">
                    <h3 className="text-sm font-semibold text-gray-300 mb-2 uppercase tracking-wider text-center">{t('winLossRatio')}</h3>
                    <ResponsiveContainer width="100%" height="100%">
                        <PieChart>
                            <Pie
                                data={donutData}
                                innerRadius={60}
                                outerRadius={80}
                                paddingAngle={5}
                                dataKey="value"
                            >
                                {donutData.map((entry, index) => (
                                    <Cell key={`cell-${index}`} fill={COLORS[index % COLORS.length]} />
                                ))}
                                <Label value={winRate} position="center" fill="#9CA3AF" style={{ fontSize: '20px', fontWeight: 'bold' }} />
                            </Pie>
                            <Tooltip contentStyle={{ backgroundColor: 'var(--color-darkCard)', borderColor: 'var(--color-gray-800)', color: 'var(--color-gray-200)' }} formatter={(value: any) => [value, 'Trades']} />
                        </PieChart>
                    </ResponsiveContainer>
                 </div>

                 {/* Average Win vs Loss */}
                 <div className="bg-darkCard border border-gray-800 rounded-xl p-4 md:p-5 h-[250px] transition-colors duration-300">
                    <h3 className="text-sm font-semibold text-gray-300 mb-2 uppercase tracking-wider text-center">{t('avgWinLoss')}</h3>
                    <ResponsiveContainer width="100%" height="100%">
                        <BarChart data={barData} layout="vertical" margin={{ top: 20, right: 30, left: 20, bottom: 5 }}>
                            <CartesianGrid strokeDasharray="3 3" stroke="#374151" horizontal={false} />
                            <XAxis type="number" stroke="#9CA3AF" tickFormatter={(value) => `${currency}${value}`} tick={{fontSize: 10}} />
                            <YAxis dataKey="name" type="category" hide />
                            <Tooltip cursor={{fill: 'var(--color-gray-800)'}} contentStyle={{ backgroundColor: 'var(--color-darkCard)', borderColor: 'var(--color-gray-800)', color: 'var(--color-gray-200)' }} formatter={(value: any) => [`${currency}${Number(value).toFixed(2)}`]} />
                            <ReferenceLine x={0} stroke="#4B5563" />
                            <Bar dataKey="win" fill="var(--profit)" radius={[0, 4, 4, 0]} barSize={30} />
                            <Bar dataKey="loss" fill="var(--loss)" radius={[4, 0, 0, 4]} barSize={30} />
                        </BarChart>
                    </ResponsiveContainer>
                 </div>
            </div>
        </div>
    );
}

// Long currency figures (e.g. "IDR-3,298,001.48") would wrap onto a second line
// inside a narrow 4-column card. Step the font down by string length so the value
// stays on one line across account currencies; break-words is the last-resort net.
function statValueSizeClass(text: string): string {
    const len = text.length;
    if (len <= 9) return 'text-3xl';
    if (len <= 12) return 'text-2xl';
    if (len <= 15) return 'text-xl';
    if (len <= 18) return 'text-lg';
    return 'text-base';
}

function StatCard({ title, value, isPositive }: { title: string, value: string | number, isPositive?: boolean }) {
    const text = String(value);
    return (
        <div className="bg-darkCard border border-gray-800 rounded-xl p-5 shadow-sm min-w-0">
            <h3 className="text-sm text-gray-400 font-medium mb-1 truncate">{title}</h3>
            <p
                title={text}
                className={`${statValueSizeClass(text)} font-bold tabular-nums leading-tight break-words ${
                    isPositive === true ? 'text-brandGreen' :
                    isPositive === false ? 'text-brandRed' : 'text-white'
                }`}
            >
                {value}
            </p>
        </div>
    );
}
