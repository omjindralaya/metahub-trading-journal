// Format a monetary amount with locale-aware thousands separators, prefixed by
// the account currency symbol (e.g. "$", "IDR"). Traders read PnL to the cent,
// so we group digits for legibility instead of abbreviating the figure. Width
// is handled at the display layer (adaptive font size), not by dropping digits.
export function formatMoney(currency: string, amount: number, lang: string): string {
    const locale = lang === 'id' ? 'id-ID' : 'en-GB';
    const sign = amount < 0 ? '-' : '';
    const body = Math.abs(amount).toLocaleString(locale, {
        minimumFractionDigits: 2,
        maximumFractionDigits: 2,
    });
    // Sign sits after the symbol to preserve the app's existing "IDR-123.45" look.
    return `${currency}${sign}${body}`;
}
