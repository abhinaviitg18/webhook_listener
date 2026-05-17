import React, { createContext, useContext, useState, useEffect } from 'react';

const AuthContext = createContext();
const DEFAULT_APP_PROFILE = {
    plan: 'basic',
    deployment_mode: 'multitenant',
    auth_mode: 'scalekit',
    public_base_url: 'https://app.agenthook.store',
    mail_domain: 'app.agenthook.store',
    docs_path: '/app?tab=docs',
    home_docs_anchor: '/#docs',
};

export function AuthProvider({ children }) {
    const [user, setUser] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [isAuthenticated, setIsAuthenticated] = useState(false);
    const [appProfile, setAppProfile] = useState(DEFAULT_APP_PROFILE);

    const fetchAppProfile = async () => {
        const response = await fetch('/api/app-profile', {
            headers: { 'Accept': 'application/json' },
            credentials: 'include'
        });
        if (!response.ok) {
            return DEFAULT_APP_PROFILE;
        }
        const data = await response.json();
        const next = { ...DEFAULT_APP_PROFILE, ...(data || {}) };
        setAppProfile(next);
        return next;
    };

    const fetchUser = async () => {
        try {
            const response = await fetch('/api/me', {
                headers: { 'Accept': 'application/json' },
                credentials: 'include'
            });
            if (response.ok) {
                const data = await response.json();
                setUser(data);
                setIsAuthenticated(true);
                return data;
            }
            setUser(null);
            setIsAuthenticated(false);
            return null;
        } catch (err) {
            console.error("Auth check failed", err);
            setUser(null);
            setIsAuthenticated(false);
            return null;
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        let cancelled = false;
        const init = async () => {
            const params = new URLSearchParams(window.location.search);
            const authError = params.get('auth_error');
            if (authError) {
                setError(authError);
                window.history.replaceState({}, document.title, window.location.pathname);
            }
            try {
                await fetchAppProfile();
                if (!cancelled) {
                    await fetchUser();
                }
            } finally {
                if (!cancelled) {
                    setLoading(false);
                }
            }
        };
        init();
        return () => {
            cancelled = true;
        };
    }, []);

    const login = async (credential = '') => {
        if (appProfile.auth_mode === 'single_tenant_claim' || appProfile.auth_mode === 'single_tenant_setup_token') {
            setError(null);
            const isClaim = appProfile.auth_mode === 'single_tenant_claim';
            const response = await fetch(isClaim ? '/auth/single-tenant/claim' : '/auth/single-tenant/login', {
                method: 'POST',
                credentials: 'include',
                headers: {
                    'Accept': 'application/json',
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify(isClaim ? { claim_code: credential } : { setup_token: credential }),
            });
            const text = await response.text();
            const data = text ? JSON.parse(text) : {};
            if (!response.ok) {
                const message = data?.error || text || `Login failed with status ${response.status}`;
                setError(message);
                throw new Error(message);
            }
            if (data?.app_profile) {
                setAppProfile({ ...DEFAULT_APP_PROFILE, ...data.app_profile });
            }
            await fetchUser();
            return data;
        }
        window.location.href = '/auth/scalekit/login';
    };

    const logout = () => {
        window.location.href = '/auth/logout';
    };

    return (
        <AuthContext.Provider value={{ user, setUser, refreshUser: fetchUser, appProfile, refreshAppProfile: fetchAppProfile, isAuthenticated, loading, error, login, logout }}>
            {children}
        </AuthContext.Provider>
    );
}

export const useAuth = () => useContext(AuthContext);
