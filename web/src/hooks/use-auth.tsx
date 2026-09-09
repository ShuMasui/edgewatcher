import React, { createContext, useContext, useState, useEffect, useRef } from 'react';
import { AuthUser } from '../types/auth';
import { authService } from '../services/auth';
import { useQueryClient } from '@tanstack/react-query';

interface AuthContextType {
  user: AuthUser | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  /** Set when the Hosted UI round trip failed. Rendered by LoginPage. */
  error: string | null;
  login: () => void;
  logout: () => void;
}

const AuthContext = createContext<AuthContextType | null>(null);

interface AuthCallbackParams {
  code: string | null;
  error: string | null;
}

/**
 * Reads what Cognito appended to the redirect URI.
 *
 * This MUST be called during render, never from an effect. ProtectedRoute
 * renders <Navigate> while isAuthenticated is still false, and <Navigate>
 * rewrites window.location through history.replaceState from its own
 * effect. React runs child effects before parent effects, so AuthProvider's
 * effect fires *after* that rewrite: by then the URL is
 * "/login?redirect=%2F" and the authorization code is gone. Render happens
 * before every effect, which is what makes this the only safe point.
 */
function readAuthCallback(): AuthCallbackParams {
  const params = new URLSearchParams(window.location.search);
  return {
    code: params.get('code'),
    // Cognito reports a refused or failed federation this way rather than
    // by omitting the code, so both have to be inspected.
    error: params.get('error_description') || params.get('error'),
  };
}

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  // Captured once, during the first render. See readAuthCallback.
  const [callback] = useState<AuthCallbackParams>(readAuthCallback);

  const [user, setUser] = useState<AuthUser | null>(authService.getUser());
  const [isAuthenticated, setIsAuthenticated] = useState<boolean>(authService.isAuthenticated());
  const [error, setError] = useState<string | null>(callback.error);

  // Starts true when an exchange is pending, so ProtectedRoute shows the
  // loading state instead of redirecting. Without this the guard bounces to
  // /login on the very first render — before the exchange has had a chance
  // to run — and the user is returned to the screen they just came from.
  const [isLoading, setIsLoading] = useState<boolean>(callback.code !== null);

  const queryClient = useQueryClient();

  // An authorization code is single-use. StrictMode invokes effects twice in
  // development, and the second exchange would fail against Cognito and
  // clear the session the first one just established.
  const exchanged = useRef(false);

  useEffect(() => {
    if (!callback.code || exchanged.current) return;
    exchanged.current = true;

    authService
      .handleAuthCallback(callback.code)
      .then(() => {
        setUser(authService.getUser());
        setIsAuthenticated(authService.isAuthenticated());
        setError(null);
      })
      .catch((err) => {
        console.error('OAuth callback failed:', err);
        // Surfaced rather than swallowed: a silent failure here is
        // indistinguishable from "the login button does nothing".
        setError('サインインを完了できませんでした。もう一度お試しください。');
      })
      .finally(() => {
        // The code is stripped whatever the outcome, so a reload does not
        // retry an already-consumed code.
        window.history.replaceState({}, document.title, window.location.pathname);
        setIsLoading(false);
      });
  }, [callback.code]);

  const login = () => {
    setError(null);
    authService.login();
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
        error,
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
