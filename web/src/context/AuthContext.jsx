import React, { createContext, useContext, useState, useEffect } from 'react';

const AuthContext = createContext();
const PENDING_CODE_KEY = 'htc_pending_code';

export function AuthProvider({ children }) {
    const [user, setUser] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [token, setToken] = useState(localStorage.getItem('htc_token'));

    useEffect(() => {
        // Keep existing logged-in session if present. Only fall back to the callback code when needed.
        const params = new URLSearchParams(window.location.search);
        const code = params.get('code');
        const authError = params.get('auth_error');

        if (authError) {
            setError(authError);
        }
        if (!code) return;

        sessionStorage.setItem(PENDING_CODE_KEY, code);
        const existingToken = localStorage.getItem('htc_token');
        if (!existingToken) {
            localStorage.setItem('htc_token', code);
            setToken(code);
        }

        window.history.replaceState({}, document.title, window.location.pathname);
    }, []);

    useEffect(() => {
        if (!token) {
            setUser(null);
            setLoading(false);
            return;
        }

        // Fetch user info from backend
        fetch('/api/me', {
            headers: { 'Authorization': `Bearer ${token}` }
        })
            .then(res => {
                if (res.ok) return res.json();
                throw new Error('Unauthorized');
            })
            .then(data => {
                setUser(data);
            })
            .catch(() => {
                const pendingCode = sessionStorage.getItem(PENDING_CODE_KEY);
                if (pendingCode && pendingCode !== token) {
                    sessionStorage.removeItem(PENDING_CODE_KEY);
                    localStorage.setItem('htc_token', pendingCode);
                    setToken(pendingCode);
                    return;
                }
                sessionStorage.removeItem(PENDING_CODE_KEY);
                setUser(null);
                localStorage.removeItem('htc_token');
                setToken(null);
            })
            .finally(() => {
                setLoading(false);
            });
    }, [token]);

    const login = () => {
        window.location.href = '/auth/scalekit/login';
    };

    const logout = () => {
        localStorage.removeItem('htc_token');
        setToken(null);
        setUser(null);
        setError(null);
    };

    return (
        <AuthContext.Provider value={{ user, token, loading, error, login, logout }}>
            {children}
        </AuthContext.Provider>
    );
}

export const useAuth = () => useContext(AuthContext);
