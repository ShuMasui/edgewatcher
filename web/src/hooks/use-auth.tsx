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
  /** Echoed back by Cognito; compared against what this tab issued. */
  state: string | null;
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
    state: params.get('state'),
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
      .handleAuthCallback(callback.code, callback.state)
      .then(() => {
        setUser(authService.getUser());
        setIsAuthenticated(authService.isAuthenticated());
        setError(null);
      })
      .catch((err) => {
        // The reason stays in the console and out of the screen. A refused
        // state and an expired code call for the same action from the
        // person in front of it — press the button again — while telling
        // them which one happened would describe the check to whoever
        // triggered it.
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
    // login() became async when PKCE landed (crypto.subtle.digest). The
    // caller is a click handler, so it is not awaited — but the rejection
    // has to go somewhere, or a failure to even start the redirect would be
    // an unhandled rejection and a button that silently does nothing.
    authService.login().catch((err) => {
      console.error('Failed to start the sign-in redirect:', err);
      setError('サインインを開始できませんでした。もう一度お試しください。');
    });
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
