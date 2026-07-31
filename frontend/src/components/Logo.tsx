import logoUrl from '../assets/images/logo.svg';

// Fox logo + wordmark, mirroring the web frontend's Warm Fox brand.
export function Logo({ size = 34, withWordmark = false }: { size?: number; withWordmark?: boolean }) {
    return (
        <span className="inline-flex items-center gap-2.5">
            <img src={logoUrl} alt="MetaHub Logo" width={size} height={size} className="object-contain" />
            {withWordmark && (
                <span className="text-xl font-extrabold tracking-tight text-white">
                    Meta<span className="text-brand">Hub</span>
                </span>
            )}
        </span>
    );
}
