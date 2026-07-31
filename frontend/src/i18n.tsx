import React, { createContext, useContext, useState, useEffect } from 'react';

const dict = {
    en: {
        tagline: "Community Trading Journal",
        dashboard: "Dashboard",
        calendar: "Calendar",
        reports: "Reports",
        trades: "Trades",
        journal: "Journal",
        notebook: "Notebook",
        syncMT5: "Sync MT5",
        lastSync: "Last Sync",
        totalPnL: "Total PnL",
        totalTrades: "Total Trades",
        winTrades: "Winning Trades",
        lossTrades: "Losing Trades",
        cumulativePnL: "Cumulative P&L",
        showingActiveAccount: "Showing active account",
        recentTrades: "Recent Trades",
        winLossRatio: "Win / Loss Ratio",
        avgWinLoss: "Avg Win vs Avg Loss",
        savedIdeas: "Saved Ideas & Strategy",
        writeNew: "Write New",
        article: "Article",
        idea: "Idea",
        publish: "Publish",
        delete: "Delete",
        backToCollection: "Back to Collection",
        emptyCollection: "Your collection is empty.",
        startDocumenting: "Start documenting your trading strategies here.",
        evalPlaceholder: "What's your trading evaluation for today?",
        post: "Post",
        noPosts: "No posts yet. Start evaluating your trades!",
        reply: "Reply",
        share: "Share",
        like: "Like",
        liveOpenPositions: "Live Open Positions (MT5)",
        newTrade: "New Trade",
        symbol: "Symbol",
        ticket: "Ticket",
        type: "Type",
        volume: "Volume",
        openPrice: "Open Price",
        sl: "S / L",
        tp: "T / P",
        time: "Time",
        price: "Price",
        commission: "Commission",
        swap: "Swap",
        profit: "Profit",
        change: "Change",
        expertID: "Expert ID",
        comment: "Comment",
        media: "Media",
        chart: "Chart",
        untitled: "Untitled Strategy",
        startDoc: "Start documenting your trading strategies here.",
        articleTitle: "Article Title...",
        tellStory: "Tell your story or write your strategy here...",
        today: "Today",
        days: "days",
        custom: "Custom",
        apply: "Apply",
        profitFactor: "Profit Factor",
        avgRR: "Avg R:R",
        expectancy: "Expectancy",
        maxDrawdown: "Max Drawdown",
        maxStreaks: "Max Streaks",
        avgHoldTime: "Avg Hold Time",
        assetPerformance: "Asset Performance",
        netPnL: "Net P&L",
        winRatePct: "Win Rate %",
        performanceByDay: "Performance by Day",
        performanceByHour: "Performance by Hour",
        tradeHistory: "Trade History",
        settings: "Settings",
        autoBackfillTitle: "Auto-fill history after a tier upgrade",
        autoBackfillDesc: "When your plan's sync window widens, automatically resend the closed trades that were previously blocked by the old limit.",
        settingsSaved: "Setting saved"
    },
    id: {
        tagline: "Jurnal Trading Komunitas",
        dashboard: "Dasbor",
        calendar: "Kalender",
        reports: "Laporan",
        trades: "Riwayat",
        journal: "Jurnal",
        notebook: "Catatan",
        syncMT5: "Sinkronisasi MT5",
        lastSync: "Terakhir Sync",
        totalPnL: "Total Keuntungan",
        totalTrades: "Total Transaksi",
        winTrades: "Transaksi Untung",
        lossTrades: "Transaksi Rugi",
        cumulativePnL: "Akumulasi P&L",
        showingActiveAccount: "Menampilkan akun aktif",
        recentTrades: "Transaksi Terakhir",
        winLossRatio: "Rasio Menang / Kalah",
        avgWinLoss: "Rata-rata Untung vs Rugi",
        savedIdeas: "Kumpulan Ide & Strategi",
        writeNew: "Tulis Baru",
        article: "Artikel",
        idea: "Ide",
        publish: "Publikasikan",
        delete: "Hapus",
        backToCollection: "Kembali ke Koleksi",
        emptyCollection: "Koleksi Anda masih kosong.",
        startDocumenting: "Mulai dokumentasikan strategi trading Anda di sini.",
        evalPlaceholder: "Bagaimana evaluasi trading Anda hari ini?",
        post: "Kirim",
        noPosts: "Belum ada postingan. Mulai evaluasi trading Anda!",
        reply: "Balas",
        share: "Bagikan",
        like: "Suka",
        liveOpenPositions: "Posisi Terbuka Aktif (MT5)",
        newTrade: "Trade Baru",
        symbol: "Simbol",
        ticket: "Tiket",
        type: "Tipe",
        volume: "Volume",
        openPrice: "Harga Buka",
        sl: "S / L",
        tp: "T / P",
        time: "Waktu",
        price: "Harga",
        commission: "Komisi",
        swap: "Biaya Inap",
        profit: "Keuntungan",
        change: "Perubahan",
        expertID: "ID Ahli",
        comment: "Komentar",
        media: "Media",
        chart: "Grafik",
        untitled: "Strategi Tanpa Judul",
        startDoc: "Mulai dokumentasikan strategi trading Anda di sini.",
        articleTitle: "Judul Artikel...",
        tellStory: "Ceritakan kisah atau strategi Anda di sini...",
        today: "Hari Ini",
        days: "hari",
        custom: "Kustom",
        apply: "Terapkan",
        profitFactor: "Faktor Profit",
        avgRR: "Rata-rata R:R",
        expectancy: "Ekspektasi",
        maxDrawdown: "Maks Drawdown",
        maxStreaks: "Rekor Beruntun",
        avgHoldTime: "Rata Tahan Posisi",
        assetPerformance: "Kinerja Aset",
        netPnL: "P&L Bersih",
        winRatePct: "Persentase Menang",
        performanceByDay: "Kinerja per Hari",
        performanceByHour: "Kinerja per Jam",
        tradeHistory: "Riwayat Transaksi",
        settings: "Pengaturan",
        autoBackfillTitle: "Isi otomatis riwayat setelah upgrade tier",
        autoBackfillDesc: "Saat jendela sync paket kamu melebar, kirim ulang otomatis transaksi tertutup yang sebelumnya ditolak oleh batas lama.",
        settingsSaved: "Pengaturan tersimpan"
    }
};

type Language = 'en' | 'id';
type Theme = 'light' | 'dark';

interface AppContextType {
    lang: Language;
    setLang: (lang: Language) => void;
    theme: Theme;
    setTheme: (theme: Theme) => void;
    t: (key: keyof typeof dict.en) => string;
    searchQuery: string;
    setSearchQuery: (query: string) => void;
}

const AppContext = createContext<AppContextType | undefined>(undefined);

export const AppProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [lang, setLang] = useState<Language>('en');
    const [theme, setThemeState] = useState<Theme>(() => {
        const s = localStorage.getItem('theme');
        if (s === 'dark' || s === 'light') return s;
        return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    });
    const [searchQuery, setSearchQuery] = useState('');

    useEffect(() => {
        document.documentElement.setAttribute('data-theme', theme);
        document.documentElement.style.colorScheme = theme;
    }, [theme]);

    useEffect(() => {
        const titleText = `MetaHub : ${dict[lang].tagline}`;
        document.title = titleText;
        if ((window as any).runtime && (window as any).runtime.WindowSetTitle) {
            (window as any).runtime.WindowSetTitle(titleText);
        }
    }, [lang]);

    // Explicit pin on user toggle (first load without a pin still follows the OS).
    const setTheme = (t: Theme) => {
        localStorage.setItem('theme', t);
        setThemeState(t);
    };

    const t = (key: keyof typeof dict.en) => {
        return dict[lang][key] || key;
    };

    return (
        <AppContext.Provider value={{ lang, setLang, theme, setTheme, t, searchQuery, setSearchQuery }}>
            {children}
        </AppContext.Provider>
    );
};

export const useAppContext = () => {
    const context = useContext(AppContext);
    if (!context) throw new Error("useAppContext must be used within AppProvider");
    return context;
};
