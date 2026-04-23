import React, { createContext, useContext, useState, useEffect } from 'react';

const AuthContext = createContext();

export function AuthProvider({ children }) {
    const [user, setUser] = useState(null);
    const [loading, setLoading] = useState(true);
    const [token, setToken] = useState(localStorage.getItem('htc_token'));

    useEffect(() => {
        // Handle ScaleKit code in URL
        const params = new URLSearchParams(window.location.search);
        const code = params.get('code');
        if (code) {
            // In a real app we'd exchange it, but for now we use it as a token mock
            // or assume the backend handled it and we just need to fetch /api/me
            localStorage.setItem('htc_token', code);
            setToken(code);
            window.history.replaceState({}, document.title, "/");
        }
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
    };

    return (
        <AuthContext.Provider value={{ user, token, loading, login, logout }}>
            {children}
        </AuthContext.Provider>
    );
}

export const useAuth = () => useContext(AuthContext);
