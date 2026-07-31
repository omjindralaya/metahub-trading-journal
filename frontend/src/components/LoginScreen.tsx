import { useRef, useState } from 'react';
import { CancelGoogleLogin, LoginWithGoogle } from '../../wailsjs/go/main/App';
import toast from 'react-hot-toast';
import { extractApiError } from '../lib/apiError';
import { Logo } from './Logo';

interface LoginScreenProps {
    onSuccess: () => void;
}

// Official Google "G" mark, for a custom sign-in button per Google's branding
// guidelines (we can't render Google's own button here: their Identity
// Services script blocks itself inside an embedded app webview).
function GoogleIcon() {
    return (
        <svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true">
            <path fill="#4285F4" d="M17.64 9.2c0-.64-.06-1.25-.16-1.84H9v3.48h4.84a4.14 4.14 0 0 1-1.8 2.72v2.26h2.9c1.7-1.57 2.7-3.88 2.7-6.62z" />
            <path fill="#34A853" d="M9 18c2.43 0 4.47-.8 5.96-2.18l-2.9-2.26c-.8.54-1.83.86-3.06.86-2.35 0-4.34-1.59-5.05-3.72H.96v2.33A9 9 0 0 0 9 18z" />
            <path fill="#FBBC05" d="M3.95 10.7A5.4 5.4 0 0 1 3.67 9c0-.59.1-1.17.28-1.7V4.97H.96A9 9 0 0 0 0 9c0 1.45.35 2.83.96 4.03l2.99-2.33z" />
            <path fill="#EA4335" d="M9 3.58c1.32 0 2.51.46 3.44 1.35l2.58-2.58C13.46.89 11.43 0 9 0A9 9 0 0 0 .96 4.97l2.99 2.33C4.66 5.17 6.65 3.58 9 3.58z" />
        </svg>
    );
}

/**
 * The app's front door. Login is mandatory: nothing renders until the user is
 * signed in to MetaHub, because every account-scoped feature (sync, plan,
 * leaderboard identity) is meaningless without knowing who this is.
 *
 * Signing in does NOT imply the plan allows cloud sync — that is decided
 * separately by the entitlement banner. A free user gets the full local journal.
 *
 * Google is the only method: the email/password form is gone because the server
 * no longer has a /auth/login endpoint to send it to. Sign-up happens on first
 * Google sign-in, so there is no "register first" link either.
 */
export function LoginScreen({ onSuccess }: LoginScreenProps) {
    const [googleLoading, setGoogleLoading] = useState(false);
    // Suppresses the error toast when the pending LoginWithGoogle promise
    // rejects because the USER clicked "Batal" — that isn't a failure worth
    // complaining about, unlike a real timeout or network error.
    const googleCancelledRef = useRef(false);

    const handleGoogleLogin = async () => {
        if (googleLoading) return;
        googleCancelledRef.current = false;
        setGoogleLoading(true);
        try {
            await LoginWithGoogle();
            toast.success('Berhasil masuk ke MetaHub');
            onSuccess();
        } catch (err) {
            if (!googleCancelledRef.current) {
                toast.error(extractApiError(err) || 'Gagal masuk dengan Google');
            }
        } finally {
            setGoogleLoading(false);
        }
    };

    const handleCancelGoogleLogin = () => {
        googleCancelledRef.current = true;
        CancelGoogleLogin();
    };

    return (
        <div className="h-screen w-screen flex items-center justify-center bg-dark text-gray-200 font-sans p-4">
            <div className="w-full max-w-md">
                <div className="flex justify-center mb-8">
                    <Logo />
                </div>

                <div className="bg-darkCard border border-gray-800 rounded-xl p-6 shadow-2xl">
                    <h1 className="text-xl font-bold text-white mb-2">Masuk ke MetaHub</h1>
                    <p className="text-gray-400 text-sm mb-6">
                        Jurnal trading ini memerlukan akun MetaHub. Masuk dengan Google untuk
                        melanjutkan — akun dibuat otomatis saat pertama kali masuk.
                    </p>

                    {googleLoading ? (
                        <div className="mb-4 flex items-center justify-between rounded-md border border-gray-700 bg-dark px-3 py-2.5 text-sm text-gray-300">
                            <span>Menunggu login di browser…</span>
                            <button
                                type="button"
                                onClick={handleCancelGoogleLogin}
                                className="text-xs font-semibold text-gray-400 hover:text-white transition-colors"
                            >
                                Batal
                            </button>
                        </div>
                    ) : (
                        <button
                            type="button"
                            onClick={handleGoogleLogin}
                            autoFocus
                            className="w-full flex items-center justify-center gap-2 px-4 py-2.5 bg-white text-gray-800 text-sm font-semibold rounded hover:brightness-95 transition-all disabled:opacity-50"
                        >
                            <GoogleIcon />
                            Masuk dengan Google
                        </button>
                    )}
                </div>

                <p className="text-center text-xs text-gray-600 mt-6">
                    Jendela browser akan terbuka untuk menyelesaikan login.
                </p>
            </div>
        </div>
    );
}
