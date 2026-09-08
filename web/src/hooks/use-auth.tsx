import React, { createContext, useContext, useState, useEffect } from 'react';
import { AuthUser } from '../types/auth';
import { authService } from '../services/auth';
import { useQueryClient } from '@tanstack/react-query';

interface AuthContextType {
  user: AuthUser | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  login: () => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType | null>(null);

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [user, setUser] = useState<AuthUser | null>(authService.getUser());
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(authService.isAuthenticated());
  const [isLoading, setIsLoading] = useState<boolean>(false);
  const queryClient = useQueryClient();

  useEffect(() => {
    // Check if returning from OAuth redirect with code in URL
    const urlParams = new URLSearchParams(window.location.search);
    const code = urlParams.get('code');

    if (code) {
      setIsLoading(true);
      authService
        .handleAuthCallback(code)
        .then(() => {
          setUser(authService.getUser());
          setIsAuthenticated(authService.isAuthenticated());
          // Clean code query parameter from URL
          const cleanUrl = window.location.pathname;
          window.history.replaceState({}, document.title, cleanUrl);
        })
        .catch((err) => {
          console.error('OAuth callback failed:', err);
        })
        .finally(() => {
          setIsLoading(false);
        });
    }
  }, []);

  const login = () => {
    authService.login();
    setUser(authService.getUser());
    setIsAuthenticated(authService.isAuthenticated());
  };

  const logout = () => {
    authService.logout(queryClient);
    setUser(null);
    setIsAuthenticated(false);
  };

  return (
    <AuthContext.Provider
      value={{
        user,
        isAuthenticated,
        isLoading,
        login,
        logout,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}
