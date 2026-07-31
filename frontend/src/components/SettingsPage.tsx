import { useEffect, useState } from 'react';
import { GetAutoBackfillEnabled, SetAutoBackfillEnabled } from '../../wailsjs/go/main/App';
import { useAppContext } from '../i18n';
import toast from 'react-hot-toast';
import { extractApiError } from '../lib/apiError';

export function SettingsPage() {
    const { t } = useAppContext();
    const [autoBackfill, setAutoBackfill] = useState(true);
    const [isLoading, setIsLoading] = useState(true);
    const [isSaving, setIsSaving] = useState(false);

    useEffect(() => {
        const load = async () => {
            try {
                if ((window as any).go?.main?.App) {
                    setAutoBackfill(await GetAutoBackfillEnabled());
                }
            } finally {
                setIsLoading(false);
            }
        };
        load();
    }, []);

    const handleToggle = async () => {
        const next = !autoBackfill;
        setAutoBackfill(next); // optimistic
        setIsSaving(true);
        try {
            if ((window as any).go?.main?.App) {
                await SetAutoBackfillEnabled(next);
            }
            toast.success(t('settingsSaved'));
        } catch (err) {
            setAutoBackfill(!next); // revert on failure
            toast.error(extractApiError(err));
        } finally {
            setIsSaving(false);
        }
    };

    return (
        <div className="p-4 md:p-8 h-full overflow-y-auto">
            <h2 className="text-xl font-semibold text-white mb-6">{t('settings')}</h2>

            <div className="bg-darkCard border border-gray-800 rounded-xl p-5 max-w-2xl">
                <div className="flex items-start justify-between gap-4">
                    <div>
                        <p className="text-white font-medium">{t('autoBackfillTitle')}</p>
                        <p className="text-gray-400 text-sm mt-1">{t('autoBackfillDesc')}</p>
                    </div>
                    <button
                        onClick={handleToggle}
                        disabled={isLoading || isSaving}
                        role="switch"
                        aria-checked={autoBackfill}
                        aria-label={t('autoBackfillTitle')}
                        className={`relative inline-flex h-6 w-11 flex-shrink-0 items-center rounded-full transition-colors disabled:opacity-50 ${
                            autoBackfill ? 'bg-brand' : 'bg-gray-700'
                        }`}
                    >
                        <span
                            className={`inline-block h-4 w-4 transform rounded-full bg-white transition-transform ${
                                autoBackfill ? 'translate-x-6' : 'translate-x-1'
                            }`}
                        />
                    </button>
                </div>
            </div>
        </div>
    );
}
