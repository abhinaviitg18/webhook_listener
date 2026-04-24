import React, { createContext, useContext, useState, useEffect } from 'react';

const AuthContext = createContext();

export function AuthProvider({ children }) {
    const [user, setUser] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const [isAuthenticated, setIsAuthenticated] = useState(false);

    useEffect(() => {
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
                } else {
                    setUser(null);
                    setIsAuthenticated(false);
                }
            } catch (err) {
                console.error("Auth check failed", err);
                setUser(null);
                setIsAuthenticated(false);
            } finally {
                setLoading(false);
            }
        };

        const params = new URLSearchParams(window.location.search);
        const authError = params.get('auth_error');
        if (authError) {
            setError(authError);
            window.history.replaceState({}, document.title, window.location.pathname);
        }

        fetchUser();
    }, []);

    const login = () => {
        window.location.href = '/auth/scalekit/login';
    };

    const logout = () => {
        window.location.href = '/auth/logout';
    };

    return (
        <AuthContext.Provider value={{ user, isAuthenticated, loading, error, login, logout }}>
            {children}
        </AuthContext.Provider>
    );
}

export const useAuth = () => useContext(AuthContext);
