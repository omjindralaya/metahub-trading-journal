import { CloudOff, AlertTriangle } from 'lucide-react';
import { OpenUpgradePage } from '../../wailsjs/go/main/App';
import type { backend } from '../../wailsjs/go/models';

interface EntitlementBannerProps {
    entitlement: backend.EntitlementView | null;
}

/**
 * Persistent notice shown while cloud sync is off. Deliberately does NOT block
 * the app: the local journal and MT5 import are free, only the push to the cloud
 * is paid. A banner that blocked the UI would be selling something we give away.
 *
 * Three states that mean different things, and are worded differently on purpose:
 *  - stale:   we could not reach the server, so we do not KNOW. Not the user's fault.
 *  - expired: they paid before and it lapsed. Renew.
 *  - neither: their plan simply does not include sync. Upgrade.
 */
export function EntitlementBanner({ entitlement }: EntitlementBannerProps) {
    if (!entitlement || entitlement.can_sync_now) return null;

    const stale = entitlement.stale;

    const message = stale
        ? 'Tidak bisa memastikan paket langganan Anda ke server. Sync ke cloud dijeda sementara — jurnal lokal tetap berjalan.'
        : entitlement.expired
            ? 'Langganan Anda sudah berakhir, jadi sync ke cloud dimatikan. Jurnal lokal dan MT5 tetap berjalan.'
            : 'Paket Anda belum termasuk sync ke cloud. Jurnal lokal dan MT5 tetap berjalan seperti biasa.';

    return (
        <div
            role="status"
            className={`flex items-center gap-3 px-4 md:px-8 py-2.5 text-sm border-b shrink-0 ${
                stale
                    ? 'bg-yellow-500/10 border-yellow-500/30 text-yellow-200'
                    : 'bg-brand/10 border-brand/30 text-brand'
            }`}
        >
            {stale ? <AlertTriangle size={16} className="shrink-0" /> : <CloudOff size={16} className="shrink-0" />}
            <span className="flex-1 min-w-0">{message}</span>
            {!stale && (
                <button
                    onClick={() => OpenUpgradePage()}
                    className="shrink-0 px-3 py-1 bg-brand text-dark text-xs font-bold rounded hover:brightness-110 transition-all"
                >
                    {entitlement.expired ? 'Perpanjang' : 'Tingkatkan paket'}
                </button>
            )}
        </div>
    );
}
