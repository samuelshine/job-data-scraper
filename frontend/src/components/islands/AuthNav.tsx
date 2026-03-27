import { useState, useEffect } from 'preact/hooks';
import { fetchSession, logout } from '../../lib/api';

export default function AuthNav() {
    const [loggedIn, setLoggedIn] = useState(false);
    const [user, setUser] = useState<{ name: string } | null>(null);

    useEffect(() => {
        fetchSession()
            .then((session) => {
                setLoggedIn(!!session.authenticated);
                if (session.authenticated && session.user) {
                    setUser(session.user);
                }
            })
            .catch(() => {
                setLoggedIn(false);
            });
    }, []);

    const handleLogout = async () => {
        try {
            await logout();
        } finally {
            window.location.href = "/";
        }
    };

    const getInitials = (name: string) => {
        return name
            .split(' ')
            .map(word => word.charAt(0))
            .join('')
            .toUpperCase()
            .slice(0, 2);
    };

    if (loggedIn && user) {
        return (
            <div class="flex items-center gap-4">
                <a
                    href="/auth/profile"
                    class="inline-flex h-10 w-10 items-center justify-center rounded-full bg-[var(--jp-accent)] text-sm font-bold text-[var(--jp-bg)] hover:bg-[var(--jp-accent-dark)] transition-colors"
                    title={user.name}
                >
                    {getInitials(user.name)}
                </a>
            </div>
        );
    }

    return (
        <div class="flex items-center gap-3">
            <a
                href="/auth/login"
                class="inline-flex items-center gap-2 rounded-[1rem] px-4 py-3 text-base font-semibold tracking-[-0.03em] text-[var(--jp-text-primary)] hover:bg-[var(--jp-surface)] hover:text-[var(--jp-accent)] transition-colors"
            >
                Login
            </a>
            <a href="/auth/register" class="btn-primary">Sign Up</a>
        </div>
    );
}
