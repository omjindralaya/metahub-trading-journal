// Ekstraksi pesan error yang aman ditampilkan ke user.
//
// Sumber error bisa beragam:
//  - string bersih dari binding Wails Go (kasus utama; Go mengembalikan
//    fmt.Errorf yang oleh Wails diserialisasi jadi string di JS)
//  - Error biasa ({ message })
//  - body envelope metahub-api { success, error: { code, message, details } }
//    kalau suatu saat ada pemanggilan fetch() langsung ke backend
//
// Tanpa helper ini, merender error mentah menghasilkan "[object Object]".
export function extractApiError(err: unknown): string {
    if (typeof err === 'string') return err;
    if (err && typeof err === 'object') {
        const anyErr = err as any;
        // envelope backend { error: { message } }
        if (anyErr.error?.message) return anyErr.error.message as string;
        // Error biasa
        if (typeof anyErr.message === 'string') return anyErr.message;
    }
    return 'Terjadi kesalahan. Coba lagi.';
}

// Kode error stabil dari envelope backend (VALIDATION_ERROR, UNAUTHORIZED, ...).
// Berguna untuk branching, mis. deteksi sesi kadaluarsa. Kosong kalau tidak ada.
export function apiErrorCode(err: unknown): string {
    if (err && typeof err === 'object') {
        const code = (err as any).error?.code;
        if (typeof code === 'string') return code;
    }
    return '';
}

// Error per-field dari envelope backend, untuk ditampilkan di form.
export function fieldErrors(err: unknown): Record<string, string> {
    const details = (err as any)?.error?.details;
    if (!Array.isArray(details)) return {};
    return Object.fromEntries(details.map((d: any) => [d.field, d.issue]));
}
