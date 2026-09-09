import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

// The real env reads import.meta.env, which is empty under vitest and would
// put the app in mock mode — where handleAuthCallback is a no-op. The bug
// only exists on the real path, so the real path is what gets configured.
vi.mock('@/config/env', () => ({
  env: {
    apiBaseUrl: 'https://api.example.test',
    cognitoDomain: 'https://auth.example.test',
    cognitoClientId: 'test-client-id',
    redirectUri: 'http://localhost:3000/',
    logoutUri: 'http://localhost:3000/login',
    useMock: false,
  },
}));

const createTestQueryClient = () =>
  new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });

// A minimal, structurally valid id_token: only the payload is read.
function idToken(payload: Record<string, unknown>): string {
  const b64 = (o: unknown) =>
    btoa(JSON.stringify(o)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `${b64({ alg: 'none' })}.${b64(payload)}.sig`;
}

describe('Hosted UI OAuth callback', () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    vi.restoreAllMocks();
    // authService is a module-level singleton that reads storage once, in
    // its constructor. Clearing storage does not clear what it already
    // holds, so without this a test inherits the previous test's session.
    vi.resetModules();
  });

  afterEach(() => {
    window.history.replaceState({}, '', '/');
  });

  // Reproduces the reported bug: pressing "Google で続行" lands back on the
  // login screen instead of the dashboard.
  it('exchanges the code and lands on the dashboard, not back on /login', async () => {
    // Cognito redirects the browser here, to "/" with ?code= and the same
    // ?state= it was handed at the authorize step.
    sessionStorage.setItem('edgewatcher_oauth_state', 'state-abc');
    sessionStorage.setItem('edgewatcher_pkce_verifier', 'verifier-abc');
    window.history.replaceState({}, '', '/?code=auth-code-123&state=state-abc');

    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        id_token: idToken({ sub: 'google-sub-1', email: 'a@example.test', name: 'A' }),
        access_token: 'access-1',
        refresh_token: 'refresh-1',
        expires_in: 3600,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const { AuthProvider } = await import('@/hooks/use-auth');
    const { AppRoutes } = await import('../index');

    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <BrowserRouter>
          <AuthProvider>
            <AppRoutes />
          </AuthProvider>
        </BrowserRouter>
      </QueryClientProvider>
    );

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledWith(
        'https://auth.example.test/oauth2/token',
        expect.anything()
      );
    });

    await waitFor(() => {
      expect(screen.queryByTestId('login-page')).not.toBeInTheDocument();
    });
  });
  // The same journey as above, but the code arrives with a state this tab
  // never issued. The user must end up on the login screen WITH an
  // explanation — not signed in, and not staring at a screen that looks
  // like the button did nothing.
  it('refuses a forged callback and explains itself on the login screen', async () => {
    sessionStorage.setItem('edgewatcher_oauth_state', 'state-abc');
    sessionStorage.setItem('edgewatcher_pkce_verifier', 'verifier-abc');
    window.history.replaceState({}, '', '/?code=attacker-code&state=attacker-state');

    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);

    const { AuthProvider } = await import('@/hooks/use-auth');
    const { AppRoutes } = await import('../index');

    render(
      <QueryClientProvider client={createTestQueryClient()}>
        <BrowserRouter>
          <AuthProvider>
            <AppRoutes />
          </AuthProvider>
        </BrowserRouter>
      </QueryClientProvider>
    );

    await waitFor(() => {
      expect(screen.getByTestId('login-error')).toBeInTheDocument();
    });

    expect(fetchMock).not.toHaveBeenCalled();
    expect(localStorage.getItem('edgewatcher_auth_tokens')).toBeNull();
  });
});
