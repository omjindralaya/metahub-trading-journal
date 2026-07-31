import { useEffect, useState } from 'react';
import { GetDashboardData, GetCurrency } from '../../wailsjs/go/main/App';
import { useAppContext } from '../i18n';
import { PieChart, Pie, Cell, ResponsiveContainer, Tooltip, BarChart, Bar, XAxis, YAxis, CartesianGrid, ComposedChart, Line } from 'recharts';

export function TradesPage({ period }: { period: string }) {
    const { t, lang, searchQuery } = useAppContext();
    const [trades, setTrades] = useState<any[]>([]);
    const [currency, setCurrency] = useState<string>("$");
    const [currentPage, setCurrentPage] = useState(1);
    const itemsPerPage = 12;

    useEffect(() => {
        const load = async () => {
            if ((window as any).go?.main?.App) {
                setTrades(await GetDashboardData(period) || []);
                setCurrency(await GetCurrency() || "$");
            }
        };
        load();
        setCurrentPage(1); // Reset page on period change
    }, [period]);

    // Pagination logic
    const filteredTrades = trades.filter(t => t.symbol.toLowerCase().includes(searchQuery.toLowerCase()));
    const totalPages = Math.ceil(filteredTrades.length / itemsPerPage) || 1;
    const startIndex = (currentPage - 1) * itemsPerPage;
    const currentTrades = filteredTrades.slice(startIndex, startIndex + itemsPerPage);

    return (
        <div className="p-4 md:p-8 h-full overflow-y-auto flex flex-col">
            <div className="bg-darkCard border border-gray-800 rounded-xl overflow-hidden flex-1 flex flex-col transition-colors duration-300">
                <div className="flex-1 overflow-x-auto">
                    <table className="w-full text-left text-sm text-gray-400 min-w-[600px]">
                        <thead className="bg-gray-800/50 text-gray-300 text-xs uppercase font-semibold sticky top-0 transition-colors duration-300">
                            <tr>
                                <th className="px-6 py-4">{t('time')}</th>
                                <th className="px-6 py-4">{t('symbol')}</th>
                                <th className="px-6 py-4">{t('type')}</th>
                                <th className="px-6 py-4 text-right">{t('profit')}</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-gray-800">
                            {currentTrades.length === 0 ? (
                                <tr><td colSpan={4} className="px-6 py-8 text-center">No trades found</td></tr>
                            ) : currentTrades.map((t, i) => (
                                <tr key={i} className="hover:bg-gray-800/20 transition-colors">
                                    <td className="px-6 py-4">{new Date(t.close_time).toLocaleString(lang === 'id' ? 'id-ID' : 'en-GB', { year: 'numeric', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false })}</td>
                                    <td className="px-6 py-4 font-medium text-white flex items-center">
                                        {t.symbol}
                                        {t.account_type && t.account_type !== "Unknown" && (
                                            <span className={`ml-2 px-1.5 py-0.5 rounded text-[10px] uppercase font-bold tracking-wider ${t.account_type === 'Real' ? 'bg-brand/10 text-brand dark:bg-brand/20 dark:text-brand border border-brand/20 dark:border-brand/30' : 'bg-brandGreen/10 text-brandGreen dark:bg-brandGreen/20 dark:text-brandGreen border border-brandGreen/20 dark:border-brandGreen/30'}`}>
                                                {t.account_type}
                                            </span>
                                        )}
                                    </td>
                                    <td className="px-6 py-4">
                                        <span className={`px-2 py-1 rounded text-xs font-semibold ${t.type.toLowerCase() === 'buy' ? 'bg-brandGreen/10 text-brandGreen dark:bg-brandGreen/20 dark:text-brandGreen' : 'bg-brandRed/10 text-brandRed dark:bg-brandRed/20 dark:text-brandRed'}`}>
                                            {t.type}
                                        </span>
                                    </td>
                                    <td className={`px-6 py-4 text-right font-semibold ${t.net_profit >= 0 ? 'text-brandGreen' : 'text-brandRed'}`}>
                                        {currency}{t.net_profit.toFixed(2)}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
                {/* Pagination Controls */}
                <div className="bg-dark border-t border-gray-800 p-4 flex items-center justify-between">
                    <span className="text-sm text-gray-500">
                        Showing {filteredTrades.length === 0 ? 0 : startIndex + 1} to {Math.min(startIndex + itemsPerPage, filteredTrades.length)} of {filteredTrades.length} entries
                    </span>
                    <div className="flex items-center space-x-2 text-sm text-gray-400">
                        <button 
                            disabled={currentPage === 1} 
                            onClick={() => setCurrentPage(p => p - 1)}
                            className="px-3 py-1 rounded border border-gray-700 hover:bg-gray-800 disabled:opacity-50 transition-colors"
                        >
                            Previous
                        </button>
                        <span className="px-3 py-1">Page {currentPage} of {totalPages}</span>
                        <button 
                            disabled={currentPage === totalPages} 
                            onClick={() => setCurrentPage(p => p + 1)}
                            className="px-3 py-1 rounded border border-gray-700 hover:bg-gray-800 disabled:opacity-50 transition-colors"
                        >
                            Next
                        </button>
                    </div>
                </div>
            </div>
        </div>
    );
}

export function CalendarPage({ period }: { period: string }) {
    const { lang, t } = useAppContext();
    const [trades, setTrades] = useState<any[]>([]);
    const [currency, setCurrency] = useState<string>("$");
    const [currentDate, setCurrentDate] = useState(new Date());
    const [selectedDate, setSelectedDate] = useState<string | null>(null);
    const [isModalOpen, setIsModalOpen] = useState(false);

    useEffect(() => {
        const load = async () => {
            if ((window as any).go?.main?.App) {
                setTrades(await GetDashboardData(period) || []);
                setCurrency(await GetCurrency() || "$");
            }
        };
        load();
    }, [period]);

    // Group trades by date
    const dailyPnL: Record<string, number> = {};
    trades.forEach(t => {
        const d = new Date(t.close_time);
        const dateKey = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
        dailyPnL[dateKey] = (dailyPnL[dateKey] || 0) + t.net_profit;
    });

    // Calendar logic
    const year = currentDate.getFullYear();
    const month = currentDate.getMonth(); // 0-11
    const firstDayOfMonth = new Date(year, month, 1).getDay(); // 0 (Sun) - 6 (Sat)
    const daysInMonth = new Date(year, month + 1, 0).getDate();

    const monthNames = lang === 'id' 
        ? ["Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"]
        : ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];
    const dayNames = lang === 'id' 
        ? ["Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"]
        : ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

    // Generate grid cells
    const cells = [];
    for (let i = 0; i < firstDayOfMonth; i++) {
        cells.push(<div key={`empty-${i}`} className="p-2 border border-gray-800/50 bg-gray-900/20 rounded opacity-50"></div>);
    }

    for (let day = 1; day <= daysInMonth; day++) {
        const dateStr = `${year}-${String(month + 1).padStart(2, '0')}-${String(day).padStart(2, '0')}`;
        const pnl = dailyPnL[dateStr];
        const hasTrade = pnl !== undefined;
        const isProfit = pnl >= 0;

        const dayOfWeek = (firstDayOfMonth + day - 1) % 7;
        const isWeekend = dayOfWeek === 0 || dayOfWeek === 6;

        cells.push(
            <div key={`day-${day}`} onClick={() => { if(hasTrade) { setSelectedDate(dateStr); setIsModalOpen(true); } }} className={`p-3 min-h-[100px] border border-gray-800 rounded flex flex-col transition-colors ${hasTrade ? 'cursor-pointer' : ''} ${
                hasTrade ? (isProfit ? 'bg-brandGreen/10 border-brandGreen/30 hover:bg-brandGreen/20' : 'bg-brandRed/10 border-brandRed/30 hover:bg-brandRed/20') : 
                (isWeekend ? 'bg-gray-900/40 border-gray-800' : 'bg-darkCard hover:bg-gray-800/50')
            }`}>
                <span className="text-gray-400 text-sm mb-auto font-medium">{day}</span>
                {hasTrade ? (
                    <span 
                        className={`text-xs md:text-sm font-bold text-right mt-2 break-all ${isProfit ? 'text-brandGreen' : 'text-brandRed'}`}
                        title={`${currency}${pnl.toFixed(2)}`}
                    >
                        {currency}{pnl.toFixed(2)}
                    </span>
                ) : (
                    isWeekend && <span className="text-[10px] md:text-xs text-gray-600 text-center mt-auto mb-auto leading-tight">{lang === 'id' ? 'Pasar Tutup' : 'Market Closed'}</span>
                )}
            </div>
        );
    }

    const prevMonth = () => setCurrentDate(new Date(year, month - 1, 1));
    const nextMonth = () => setCurrentDate(new Date(year, month + 1, 1));

    return (
        <div className="p-8 h-[calc(100vh-64px)] overflow-y-auto">
            <div className="flex items-center justify-between mb-6">
                <h2 className="text-xl font-semibold text-white">
                    {monthNames[month]} {year}
                </h2>
                <div className="flex gap-2">
                    <button onClick={prevMonth} className="px-4 py-2 bg-darkCard text-white border border-gray-800 rounded hover:bg-gray-800 transition-colors">
                        &lt; {lang === 'id' ? 'Seb' : 'Prev'}
                    </button>
                    <button onClick={nextMonth} className="px-4 py-2 bg-darkCard text-white border border-gray-800 rounded hover:bg-gray-800 transition-colors">
                        {lang === 'id' ? 'Lanjut' : 'Next'} &gt;
                    </button>
                </div>
            </div>

            <div className="grid grid-cols-7 gap-3 mb-2">
                {dayNames.map(day => (
                    <div key={day} className="text-center text-sm font-semibold text-gray-500 uppercase tracking-wider">{day}</div>
                ))}
            </div>

            <div className="grid grid-cols-7 gap-3">
                {cells}
            </div>

            {isModalOpen && selectedDate && (
                <CalendarDayModal 
                    date={selectedDate} 
                    trades={trades.filter(t => {
                        const d = new Date(t.close_time);
                        const tDate = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
                        return tDate === selectedDate;
                    })}
                    onClose={() => setIsModalOpen(false)} 
                    currency={currency}
                />
            )}
        </div>
    );
}

// === REUSABLE: Modal Wrapper ===
function Modal({ onClose, title, children }: { onClose: () => void, title: string, children: React.ReactNode }) {
    return (
        <div className="fixed inset-0 bg-black/60 backdrop-blur-sm z-[100] flex items-center justify-center p-4" onClick={onClose}>
            <div className="bg-darkCard border border-gray-800 rounded-2xl p-6 w-full max-w-5xl max-h-[90vh] overflow-y-auto shadow-2xl flex flex-col animate-fade-in relative" onClick={e => e.stopPropagation()}>
                <div className="flex justify-between items-center mb-6">
                    <h2 className="text-2xl font-bold text-white">{title}</h2>
                    <button
                        onClick={onClose}
                        className="text-gray-400 hover:text-white transition-all bg-gray-800/50 hover:bg-brandRed/80 w-10 h-10 rounded-full flex items-center justify-center flex-shrink-0 group"
                        aria-label="Close"
                    >
                        <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round" className="transition-transform group-hover:scale-110">
                            <line x1="18" y1="6" x2="6" y2="18"></line>
                            <line x1="6" y1="6" x2="18" y2="18"></line>
                        </svg>
                    </button>
                </div>
                {children}
            </div>
        </div>
    );
}

// === REUSABLE: Trade Analytics Helper ===
function useTradeAnalytics(trades: any[]) {
    let grossProfit = 0;
    let grossLoss = 0;
    let maxDrawdown = 0;
    let peak = 0;
    let equity = 0;
    let currentWinStreak = 0;
    let maxWinStreak = 0;
    let currentLossStreak = 0;
    let maxLossStreak = 0;

    const sorted = [...trades].sort((a, b) => new Date(a.close_time).getTime() - new Date(b.close_time).getTime());

    sorted.forEach(t => {
        if (t.net_profit > 0) {
            grossProfit += t.net_profit;
            currentWinStreak++;
            maxWinStreak = Math.max(maxWinStreak, currentWinStreak);
            currentLossStreak = 0;
        } else if (t.net_profit <= 0) {
            grossLoss += Math.abs(t.net_profit);
            if (t.net_profit < 0) {
                currentLossStreak++;
                maxLossStreak = Math.max(maxLossStreak, currentLossStreak);
            }
            currentWinStreak = 0;
        }
        equity += t.net_profit;
        if (equity > peak) peak = equity;
        const dd = peak - equity;
        if (dd > maxDrawdown) maxDrawdown = dd;
    });

    const profitFactor = grossLoss > 0 ? (grossProfit / grossLoss).toFixed(2) : (grossProfit > 0 ? 'Max' : '0.00');
    const winTrades = sorted.filter(t => t.net_profit > 0);
    const lossTrades = sorted.filter(t => t.net_profit < 0);
    const avgWin = winTrades.length > 0 ? grossProfit / winTrades.length : 0;
    const avgLoss = lossTrades.length > 0 ? grossLoss / lossTrades.length : 0;
    const avgRR = avgLoss > 0 ? (avgWin / avgLoss).toFixed(2) : (avgWin > 0 ? 'Max' : '0.00');

    return { sorted, profitFactor, avgRR, maxDrawdown, maxWinStreak, maxLossStreak, grossProfit, grossLoss, avgWin, avgLoss, winTrades, lossTrades };
}

// === REUSABLE: Trade Analytics Stats Grid ===
function TradeAnalyticsGrid({ trades, currency }: { trades: any[], currency: string }) {
    const { t } = useAppContext();
    const { profitFactor, avgRR, maxDrawdown, maxWinStreak, maxLossStreak } = useTradeAnalytics(trades);

    return (
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
            <ReportStatCard title={t('profitFactor')} value={profitFactor} valueClass={Number(profitFactor) >= 1.5 ? 'text-brandGreen' : Number(profitFactor) >= 1 ? 'text-brand' : 'text-brandRed'} />
            <ReportStatCard title={t('avgRR')} value={`1 : ${avgRR}`} valueClass="text-white" />
            <ReportStatCard title={t('maxDrawdown')} value={`${currency}${maxDrawdown.toFixed(2)}`} valueClass="text-brandRed" />
            <div className="bg-dark border border-gray-800 rounded-xl p-3 flex flex-col justify-center shadow-inner">
                <span className="text-gray-400 text-[10px] md:text-xs font-semibold uppercase tracking-wider mb-1 truncate">{t('maxStreaks')}</span>
                <div className="flex items-center gap-2 text-base md:text-lg font-bold">
                    <span className="text-brandGreen">{maxWinStreak}W</span>
                    <span className="text-gray-600">/</span>
                    <span className="text-brandRed">{maxLossStreak}L</span>
                </div>
            </div>
        </div>
    );
}

// === REUSABLE: Trade Table with Pagination ===
function TradeTablePaginated({ trades, currency, itemsPerPage = 5 }: { trades: any[], currency: string, itemsPerPage?: number }) {
    const { t, lang } = useAppContext();
    const [currentPage, setCurrentPage] = useState(1);

    const sorted = [...trades].sort((a, b) => new Date(a.close_time).getTime() - new Date(b.close_time).getTime());
    const totalPages = Math.ceil(sorted.length / itemsPerPage) || 1;
    const startIndex = (currentPage - 1) * itemsPerPage;
    const currentTrades = sorted.slice(startIndex, startIndex + itemsPerPage);

    return (
        <div className="bg-dark border border-gray-800 rounded-xl overflow-hidden flex-1 flex flex-col shadow-inner">
            <div className="overflow-x-auto flex-1">
                <table className="w-full text-left text-sm text-gray-400 min-w-[600px]">
                    <thead className="bg-gray-800/80 text-gray-300 text-xs uppercase font-semibold sticky top-0">
                        <tr>
                            <th className="px-4 py-3">{t('time')}</th>
                            <th className="px-4 py-3">{t('symbol')}</th>
                            <th className="px-4 py-3">{t('type')}</th>
                            <th className="px-4 py-3 text-right">{t('profit')}</th>
                        </tr>
                    </thead>
                    <tbody className="divide-y divide-gray-800">
                        {currentTrades.length === 0 ? (
                            <tr><td colSpan={4} className="px-4 py-8 text-center text-gray-500">{t('noPosts') || 'No trades'}</td></tr>
                        ) : currentTrades.map((tr, i) => (
                            <tr key={i} className="hover:bg-gray-800/40 transition-colors">
                                <td className="px-4 py-3 text-gray-300">{new Date(tr.close_time).toLocaleTimeString(lang === 'id' ? 'id-ID' : 'en-GB', { hour: '2-digit', minute: '2-digit', hour12: false })}</td>
                                <td className="px-4 py-3 font-medium text-white flex items-center">
                                    {tr.symbol}
                                    {tr.account_type && tr.account_type !== "Unknown" && (
                                        <span className={`ml-2 px-1.5 py-0.5 rounded text-[9px] uppercase font-bold tracking-wider ${tr.account_type === 'Real' ? 'bg-brand/10 text-brand dark:bg-brand/20 dark:text-brand border border-brand/20 dark:border-brand/30' : 'bg-brandGreen/10 text-brandGreen dark:bg-brandGreen/20 dark:text-brandGreen border border-brandGreen/20 dark:border-brandGreen/30'}`}>
                                            {tr.account_type}
                                        </span>
                                    )}
                                </td>
                                <td className="px-4 py-3">
                                    <span className={`px-2 py-0.5 rounded text-[10px] font-bold uppercase tracking-wider ${tr.type.toLowerCase() === 'buy' ? 'bg-brandGreen/10 text-brandGreen dark:bg-brandGreen/20 dark:text-brandGreen border border-brandGreen/30' : 'bg-brandRed/10 text-brandRed dark:bg-brandRed/20 dark:text-brandRed border border-brandRed/30'}`}>
                                        {tr.type}
                                    </span>
                                </td>
                                <td className={`px-4 py-3 text-right font-bold tracking-wide ${tr.net_profit >= 0 ? 'text-brandGreen' : 'text-brandRed'}`}>
                                    {currency}{tr.net_profit.toFixed(2)}
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
            
            {totalPages > 1 && (
                <div className="bg-gray-800/40 border-t border-gray-800 p-3 flex items-center justify-between text-xs font-medium">
                    <span className="text-gray-500">
                        Showing {startIndex + 1} to {Math.min(startIndex + itemsPerPage, sorted.length)} of {sorted.length} entries
                    </span>
                    <div className="flex items-center space-x-2">
                        <button disabled={currentPage === 1} onClick={() => setCurrentPage(p => p - 1)} className="px-3 py-1.5 rounded border border-gray-700 text-gray-400 hover:bg-gray-700 hover:text-white disabled:opacity-30 disabled:hover:bg-transparent transition-all">Prev</button>
                        <span className="text-gray-400 px-2">{currentPage} / {totalPages}</span>
                        <button disabled={currentPage === totalPages} onClick={() => setCurrentPage(p => p + 1)} className="px-3 py-1.5 rounded border border-gray-700 text-gray-400 hover:bg-gray-700 hover:text-white disabled:opacity-30 disabled:hover:bg-transparent transition-all">Next</button>
                    </div>
                </div>
            )}
        </div>
    );
}

// === Calendar Day Modal (uses reusable components) ===
function CalendarDayModal({ date, trades, onClose, currency }: { date: string, trades: any[], onClose: () => void, currency: string }) {
    const { t, lang } = useAppContext();
    const title = new Date(date).toLocaleDateString(lang === 'id' ? 'id-ID' : 'en-GB', { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' });

    return (
        <Modal onClose={onClose} title={title}>
            <TradeAnalyticsGrid trades={trades} currency={currency} />
            <h3 className="text-lg font-semibold text-white mb-4 border-b border-gray-800 pb-2">{t('tradeHistory')}</h3>
            <TradeTablePaginated trades={trades} currency={currency} itemsPerPage={5} />
        </Modal>
    );
}
export function ReportsPage({ period }: { period: string }) {
    const { t, lang, searchQuery } = useAppContext();
    const [trades, setTrades] = useState<any[]>([]);
    const [currency, setCurrency] = useState<string>("$");

    useEffect(() => {
        const load = async () => {
            if ((window as any).go?.main?.App) {
                setTrades(await GetDashboardData(period) || []);
                setCurrency(await GetCurrency() || "$");
            }
        };
        load();
    }, [period]);

    const filteredTrades = trades.filter(t => t.symbol.toLowerCase().includes(searchQuery.toLowerCase()));

    // Analytics Logic
    let grossProfit = 0;
    let grossLoss = 0;
    let maxDrawdown = 0;
    let peak = 0;
    let equity = 0;
    let currentWinStreak = 0;
    let maxWinStreak = 0;
    let currentLossStreak = 0;
    let maxLossStreak = 0;
    let totalHoldingTimeMs = 0;
    let closedTradesWithTime = 0;

    const symbolMap: Record<string, { pnl: number, wins: number, total: number }> = {};
    const dayMap: Record<string, number> = { 'Mon': 0, 'Tue': 0, 'Wed': 0, 'Thu': 0, 'Fri': 0 };
    const hourMap: Record<string, number> = {};
    for (let i = 0; i < 24; i++) {
        hourMap[i.toString().padStart(2, '0')] = 0;
    }

    // Sort chronologically for accurate drawdown/streaks
    const chronologicalTrades = [...filteredTrades].sort((a, b) => new Date(a.close_time).getTime() - new Date(b.close_time).getTime());

    chronologicalTrades.forEach(t => {
        // Gross Profit/Loss & Streaks
        if (t.net_profit > 0) {
            grossProfit += t.net_profit;
            currentWinStreak++;
            maxWinStreak = Math.max(maxWinStreak, currentWinStreak);
            currentLossStreak = 0;
        } else if (t.net_profit <= 0) {
            grossLoss += Math.abs(t.net_profit);
            if (t.net_profit < 0) {
                currentLossStreak++;
                maxLossStreak = Math.max(maxLossStreak, currentLossStreak);
            }
            currentWinStreak = 0;
        }

        // Drawdown
        equity += t.net_profit;
        if (equity > peak) {
            peak = equity;
        }
        const drawdown = peak - equity;
        if (drawdown > maxDrawdown) {
            maxDrawdown = drawdown;
        }

        // Holding time calculation
        if (t.open_time && t.close_time) {
            const tIn = new Date(t.open_time).getTime();
            const tOut = new Date(t.close_time).getTime();
            if (!isNaN(tIn) && !isNaN(tOut) && tOut > tIn) {
                totalHoldingTimeMs += (tOut - tIn);
                closedTradesWithTime++;
            }
        }

        // Symbol Analysis
        if (!symbolMap[t.symbol]) {
            symbolMap[t.symbol] = { pnl: 0, wins: 0, total: 0 };
        }
        symbolMap[t.symbol].pnl += t.net_profit;
        symbolMap[t.symbol].total += 1;
        if (t.net_profit > 0) symbolMap[t.symbol].wins += 1;

        // Day of Week
        const dateObj = new Date(t.close_time);
        const days = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
        const dayName = days[dateObj.getDay()];
        if (dayMap[dayName] !== undefined) {
            dayMap[dayName] += t.net_profit;
        }

        // Hour of Day
        const hour = dateObj.getHours().toString().padStart(2, '0');
        if (hourMap[hour] !== undefined) {
            hourMap[hour] += t.net_profit;
        }
    });

    const profitFactor = grossLoss > 0 ? (grossProfit / grossLoss).toFixed(2) : (grossProfit > 0 ? 'Max' : '0.00');
    
    // Average RR & Expectancy
    const winTrades = chronologicalTrades.filter(t => t.net_profit > 0);
    const lossTrades = chronologicalTrades.filter(t => t.net_profit < 0);
    const avgWin = winTrades.length > 0 ? grossProfit / winTrades.length : 0;
    const avgLoss = lossTrades.length > 0 ? grossLoss / lossTrades.length : 0;
    const avgRR = avgLoss > 0 ? (avgWin / avgLoss).toFixed(2) : (avgWin > 0 ? 'Max' : '0.00');
    const winRate = chronologicalTrades.length > 0 ? (winTrades.length / chronologicalTrades.length) : 0;
    const expectancy = (winRate * avgWin) - ((1 - winRate) * avgLoss);

    // Format Holding Time
    let avgHoldingTimeStr = '-';
    if (closedTradesWithTime > 0) {
        const avgMs = totalHoldingTimeMs / closedTradesWithTime;
        const hours = Math.floor(avgMs / (1000 * 60 * 60));
        const mins = Math.floor((avgMs % (1000 * 60 * 60)) / (1000 * 60));
        if (hours > 0) avgHoldingTimeStr = `${hours}h ${mins}m`;
        else avgHoldingTimeStr = `${mins}m`;
    }

    // Chart Data
    const symbolData = Object.keys(symbolMap).map(k => ({ 
        name: k, 
        pnl: symbolMap[k].pnl, 
        winRate: Number(((symbolMap[k].wins / symbolMap[k].total) * 100).toFixed(1))
    })).sort((a, b) => b.pnl - a.pnl);
    
    const dayData = Object.keys(dayMap).map(k => ({ name: k, pnl: dayMap[k] }));
    const hourData = Object.keys(hourMap).map(k => ({ name: k, pnl: hourMap[k] }));

    return (
        <div className="p-4 md:p-8 h-[calc(100vh-64px)] overflow-y-auto">
            {/* Advanced Stats Grid */}
            <div className="grid grid-cols-2 lg:grid-cols-3 xl:grid-cols-6 gap-3 md:gap-4 mb-6">
                <ReportStatCard 
                    title={t('profitFactor')} 
                    value={profitFactor} 
                    valueClass={Number(profitFactor) >= 1.5 ? 'text-brandGreen' : Number(profitFactor) >= 1 ? 'text-brand' : 'text-brandRed'} 
                />
                <ReportStatCard 
                    title={t('avgRR')} 
                    value={`1 : ${avgRR}`} 
                    valueClass="text-white" 
                />
                <ReportStatCard 
                    title={t('expectancy')} 
                    value={`${currency}${expectancy.toFixed(2)}`} 
                    valueClass={expectancy >= 0 ? 'text-brandGreen' : 'text-brandRed'} 
                />
                <ReportStatCard 
                    title={t('maxDrawdown')} 
                    value={`${currency}${maxDrawdown.toFixed(2)}`} 
                    valueClass="text-brandRed" 
                />
                <div className="bg-darkCard border border-gray-800 rounded-xl p-3 md:p-4 flex flex-col justify-center min-w-0">
                    <span className="text-gray-400 text-[10px] md:text-xs font-semibold uppercase tracking-wider mb-1 truncate">{t('maxStreaks')}</span>
                    <div className="flex items-center gap-2 text-base md:text-lg font-bold truncate">
                        <span className="text-brandGreen">{maxWinStreak}W</span>
                        <span className="text-gray-600">/</span>
                        <span className="text-brandRed">{maxLossStreak}L</span>
                    </div>
                </div>
                <ReportStatCard 
                    title={t('avgHoldTime')} 
                    value={avgHoldingTimeStr} 
                    valueClass="text-brand" 
                />
            </div>

            {/* Main Charts Area */}
            <div className="grid grid-cols-1 lg:grid-cols-2 gap-6 mb-6">
                
                {/* Symbol Analysis: PnL and WinRate */}
                <ReportChartCard title={t('assetPerformance')}>
                    <ComposedChart data={symbolData} margin={{ top: 20, right: 20, bottom: 20, left: 0 }}>
                        <CartesianGrid strokeDasharray="3 3" stroke="#374151" vertical={false} />
                        <XAxis dataKey="name" stroke="#9CA3AF" tick={{fontSize: 12}} />
                        <YAxis yAxisId="left" stroke="#9CA3AF" tickFormatter={(v) => `${currency}${v}`} tick={{fontSize: 12}} />
                        <YAxis yAxisId="right" orientation="right" stroke="#9CA3AF" tickFormatter={(v) => `${v}%`} tick={{fontSize: 12}} />
                        <Tooltip 
                            cursor={{fill: 'var(--color-gray-800)'}} 
                            contentStyle={{ backgroundColor: 'var(--color-darkCard)', borderColor: 'var(--color-gray-800)', color: 'var(--color-gray-200)' }} 
                        />
                        <Bar yAxisId="left" dataKey="pnl" name={t('netPnL')} fill="var(--brand)" radius={[4, 4, 0, 0]} />
                        <Line yAxisId="right" type="monotone" dataKey="winRate" name={t('winRatePct')} stroke="var(--profit)" strokeWidth={3} dot={{r: 4}} />
                    </ComposedChart>
                </ReportChartCard>

                {/* Day of Week Analysis */}
                <ReportChartCard title={t('performanceByDay')}>
                    <BarChart data={dayData}>
                        <CartesianGrid strokeDasharray="3 3" stroke="#374151" vertical={false} />
                        <XAxis dataKey="name" stroke="#9CA3AF" tick={{fontSize: 12}} />
                        <YAxis stroke="#9CA3AF" tickFormatter={(v) => `${currency}${v}`} tick={{fontSize: 12}} width={60} />
                        <Tooltip 
                            cursor={{fill: 'var(--color-gray-800)'}} 
                            contentStyle={{ backgroundColor: 'var(--color-darkCard)', borderColor: 'var(--color-gray-800)', color: 'var(--color-gray-200)' }}
                            formatter={(v: any) => [`${currency}${Number(v).toFixed(2)}`, 'PnL']}
                        />
                        <Bar dataKey="pnl" radius={[4, 4, 0, 0]}>
                            {
                                dayData.map((entry, index) => (
                                    <Cell key={`cell-${index}`} fill={entry.pnl >= 0 ? 'var(--profit)' : 'var(--loss)'} />
                                ))
                            }
                        </Bar>
                    </BarChart>
                </ReportChartCard>
            </div>

            {/* Hourly Analysis */}
            <ReportChartCard title={t('performanceByHour')}>
                <BarChart data={hourData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#374151" vertical={false} />
                    <XAxis dataKey="name" stroke="#9CA3AF" tick={{fontSize: 10}} />
                    <YAxis stroke="#9CA3AF" tickFormatter={(v) => `${currency}${v}`} tick={{fontSize: 12}} width={60} />
                    <Tooltip 
                        cursor={{fill: 'var(--color-gray-800)'}} 
                        contentStyle={{ backgroundColor: 'var(--color-darkCard)', borderColor: 'var(--color-gray-800)', color: 'var(--color-gray-200)' }}
                        formatter={(v: any) => [`${currency}${Number(v).toFixed(2)}`, 'PnL']}
                        labelFormatter={(label) => lang === 'id' ? `Jam: ${label}:00` : `Hour: ${label}:00`}
                    />
                    <Bar dataKey="pnl" radius={[2, 2, 0, 0]}>
                        {
                            hourData.map((entry, index) => (
                                <Cell key={`cell-${index}`} fill={entry.pnl >= 0 ? 'var(--profit)' : 'var(--loss)'} />
                            ))
                        }
                    </Bar>
                </BarChart>
            </ReportChartCard>
        </div>
    );
}

// === REUSABLE COMPONENTS FOR REPORTS ===

function ReportStatCard({ title, value, valueClass }: { title: string, value: string | number, valueClass: string }) {
    return (
        <div className="bg-darkCard border border-gray-800 rounded-xl p-3 md:p-4 flex flex-col justify-center min-w-0">
            <span className="text-gray-400 text-[10px] md:text-xs font-semibold uppercase tracking-wider mb-1 truncate">{title}</span>
            <span 
                className={`text-lg md:text-xl font-bold truncate ${valueClass}`}
                title={String(value)}
            >
                {value}
            </span>
        </div>
    );
}

function ReportChartCard({ title, children }: { title: string, children: React.ReactNode }) {
    return (
        <div className="bg-darkCard border border-gray-800 rounded-xl p-5 min-h-[350px] flex flex-col">
            <h3 className="text-sm font-semibold text-gray-300 mb-4 uppercase tracking-wider">{title}</h3>
            <div className="h-72 w-full">
                <ResponsiveContainer width="100%" height="100%">
                    {children as React.ReactElement}
                </ResponsiveContainer>
            </div>
        </div>
    );
}

import { AddManualTrade, FetchOpenPositions } from '../../wailsjs/go/main/App';

export function NewTradePage() {
    const { t, searchQuery } = useAppContext();
    const [form, setForm] = useState({ symbol: '', type: 'Buy', profit: 0, date: new Date().toISOString().split('T')[0] });
    const [openPositions, setOpenPositions] = useState<any[]>([]);
    const [currency, setCurrency] = useState<string>("$");
    const [isLoading, setIsLoading] = useState(false);

    useEffect(() => {
        let interval: any;
        const load = async () => {
            if ((window as any).go?.main?.App) {
                setCurrency(await GetCurrency() || "$");
                await syncPositions();

                // Auto-refresh every 5 seconds
                interval = setInterval(() => {
                    syncPositions(true); // pass true for background sync (no loading state)
                }, 5000);
            }
        };
        load();
        
        return () => {
            if (interval) clearInterval(interval);
        };
    }, []);

    const syncPositions = async (isBackground = false) => {
        if (!isBackground) setIsLoading(true);
        try {
            if ((window as any).go?.main?.App) {
                const pos = await FetchOpenPositions();
                setOpenPositions(pos || []);
            }
        } catch (e) {
            console.error("Failed to fetch open positions", e);
        }
        if (!isBackground) setIsLoading(false);
    };

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        // Manual trade logic removed as per user request
    };

    const filteredPositions = openPositions.filter(p => p.symbol.toLowerCase().includes(searchQuery.toLowerCase()));

    return (
        <div className="p-4 md:p-8 h-full overflow-y-auto max-w-7xl mx-auto flex flex-col">
            {/* Live Open Positions */}
            <div className="w-full flex flex-col">
                <div className="flex flex-col md:flex-row md:items-center justify-between gap-4 mb-6">
                    <h2 className="text-xl font-semibold text-white transition-colors duration-300">{t('liveOpenPositions')}</h2>
                    <button 
                        onClick={() => syncPositions(false)}
                        disabled={isLoading}
                        className="bg-brand text-dark font-bold px-4 py-2 rounded hover:brightness-110 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                        {isLoading ? 'Syncing...' : t('syncMT5')}
                    </button>
                </div>
                
                <div className="flex-1 bg-darkCard border border-gray-800 rounded-xl overflow-hidden transition-colors duration-300 w-full overflow-x-auto">
                    <table className="w-full text-left text-sm text-gray-400 min-w-[800px]">
                        <thead className="bg-gray-800/50 text-gray-300 text-xs uppercase font-semibold transition-colors duration-300">
                            <tr>
                                <th className="px-4 py-3">{t('symbol')}</th>
                                <th className="px-4 py-3">{t('type')}</th>
                                <th className="px-4 py-3">{t('volume')}</th>
                                <th className="px-4 py-3">{t('openPrice')}</th>
                                <th className="px-4 py-3">{t('price')}</th>
                                <th className="px-4 py-3 text-gray-500">{t('sl')}</th>
                                <th className="px-4 py-3 text-gray-500">{t('tp')}</th>
                                <th className="px-4 py-3 text-right">{t('profit')}</th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-gray-800">
                            {filteredPositions.length === 0 ? (
                                <tr><td colSpan={8} className="px-6 py-8 text-center">{t('noPosts')}</td></tr>
                            ) : filteredPositions.map((p, i) => (
                                <tr key={i} className="hover:bg-gray-800/20 transition-colors">
                                    <td className="px-4 py-3 font-medium text-white flex items-center">
                                        {p.symbol}
                                        {p.account_type && p.account_type !== "Unknown" && (
                                            <span className={`ml-2 px-1.5 py-0.5 rounded text-[10px] uppercase font-bold tracking-wider ${p.account_type === 'Real' ? 'bg-brand/10 text-brand dark:bg-brand/20 dark:text-brand border border-brand/20 dark:border-brand/30' : 'bg-brandGreen/10 text-brandGreen dark:bg-brandGreen/20 dark:text-brandGreen border border-brandGreen/20 dark:border-brandGreen/30'}`}>
                                                {p.account_type}
                                            </span>
                                        )}
                                    </td>
                                    <td className="px-4 py-3">
                                        <span className={`px-2 py-1 rounded text-xs font-semibold ${p.type.toLowerCase() === 'buy' ? 'bg-brandGreen/10 text-brandGreen dark:bg-brandGreen/20 dark:text-brandGreen' : 'bg-brandRed/10 text-brandRed dark:bg-brandRed/20 dark:text-brandRed'}`}>
                                            {p.type}
                                        </span>
                                    </td>
                                    <td className="px-4 py-3">{p.volume}</td>
                                    <td className="px-4 py-3">{p.open_price}</td>
                                    <td className="px-4 py-3">{p.current_price}</td>
                                    <td className="px-4 py-3 text-brandRed font-medium">{p.sl > 0 ? p.sl : '-'}</td>
                                    <td className="px-4 py-3 text-brandGreen font-medium">{p.tp > 0 ? p.tp : '-'}</td>
                                    <td className={`px-4 py-3 text-right font-semibold ${p.floating_pnl >= 0 ? 'text-brandGreen' : 'text-brandRed'}`}>
                                        {currency}{p.floating_pnl.toFixed(2)}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    );
}

function Placeholder({ title }: { title: string }) {
    return (
        <div className="flex flex-col items-center justify-center h-full p-8 text-center bg-dark">
            <h2 className="text-2xl font-semibold text-white mb-2">{title}</h2>
            <p className="text-gray-500 max-w-md">
                Halaman ini masih dalam tahap pengembangan (Work in Progress). 
                Fitur ini akan segera tersedia di pembaruan berikutnya!
            </p>
        </div>
    );
}
