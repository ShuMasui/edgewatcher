import { describe, it, expect, vi, beforeEach } from 'vitest';

// The real env reads import.meta.env, which is empty under vitest and would
// put the service in mock mode — where none of this code runs.
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

const STATE_KEY = 'edgewatcher_oauth_state';
const VERIFIER_KEY = 'edgewatcher_pkce_verifier';

function base64url(bytes: ArrayBuffer): string {
  return btoa(String.fromCharCode(...new Uint8Array(bytes)))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

function idToken(payload: Record<string, unknown>): string {
  const b64 = (o: unknown) =>
    btoa(JSON.stringify(o)).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
  return `${b64({ alg: 'none' })}.${b64(payload)}.sig`;
}

function stubLocation() {
  Object.defineProperty(window, 'location', {
    value: { href: '', origin: 'http://localhost:3000', pathname: '/', search: '' },
    writable: true,
    configurable: true,
  });
}

describe('authService.login — state and PKCE', () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    vi.restoreAllMocks();
    // authService is a module-level singleton that reads storage once, in
    // its constructor. Clearing storage does not clear what it already
    // holds, so without this a test inherits the previous test's session.
    vi.resetModules();
    stubLocation();
  });

  it('sends state and an S256 challenge, and keeps the secrets in sessionStorage', async () => {
    const { authService } = await import('@/services/auth');
    await authService.login();

    const url = new URL(window.location.href);
    expect(url.origin + url.pathname).toBe('https://auth.example.test/oauth2/authorize');

    const state = url.searchParams.get('state');
    expect(state).toBeTruthy();
    expect(sessionStorage.getItem(STATE_KEY)).toBe(state);

    expect(url.searchParams.get('code_challenge_method')).toBe('S256');

    // The verifier itself must never travel on the authorize request — the
    // whole point of PKCE is that it stays in the browser until the
    // exchange.
    const verifier = sessionStorage.getItem(VERIFIER_KEY);
    expect(verifier).toBeTruthy();
    expect(window.location.href).not.toContain(verifier!);

    // The challenge must actually be derived from the verifier. Asserting
    // only "some string is present" would pass against a random value, which
    // Cognito would then reject at the exchange.
    const expected = base64url(
      await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier!))
    );
    expect(url.searchParams.get('code_challenge')).toBe(expected);
  });

  it('generates a fresh state and verifier on every attempt', async () => {
    const { authService } = await import('@/services/auth');

    await authService.login();
    const first = {
      state: sessionStorage.getItem(STATE_KEY),
      verifier: sessionStorage.getItem(VERIFIER_KEY),
    };

    await authService.login();
    expect(sessionStorage.getItem(STATE_KEY)).not.toBe(first.state);
    expect(sessionStorage.getItem(VERIFIER_KEY)).not.toBe(first.verifier);
  });
});

describe('authService.handleAuthCallback — state and PKCE', () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    vi.restoreAllMocks();
    // authService is a module-level singleton that reads storage once, in
    // its constructor. Clearing storage does not clear what it already
    // holds, so without this a test inherits the previous test's session.
    vi.resetModules();
    stubLocation();
  });

  function stubTokenEndpoint() {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => ({
        id_token: idToken({ sub: 'sub-1', email: 'a@example.test', name: 'A' }),
        access_token: 'access-1',
        expires_in: 3600,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);
    return fetchMock;
  }

  it('sends the stored code_verifier on the exchange', async () => {
    const { authService } = await import('@/services/auth');
    sessionStorage.setItem(STATE_KEY, 'the-state');
    sessionStorage.setItem(VERIFIER_KEY, 'the-verifier');
    const fetchMock = stubTokenEndpoint();

    await authService.handleAuthCallback('auth-code', 'the-state');

    const body = new URLSearchParams(fetchMock.mock.calls[0][1].body as string);
    expect(body.get('code_verifier')).toBe('the-verifier');
    expect(body.get('code')).toBe('auth-code');
  });

  // The CSRF assertion. An attacker who gets the browser to land on the
  // redirect URI with THEIR authorization code must not have it exchanged:
  // that would sign this user into the attacker's account.
  it('refuses a state that does not match, without contacting the token endpoint', async () => {
    const { authService } = await import('@/services/auth');
    sessionStorage.setItem(STATE_KEY, 'the-state');
    sessionStorage.setItem(VERIFIER_KEY, 'the-verifier');
    const fetchMock = stubTokenEndpoint();

    await expect(
      authService.handleAuthCallback('attacker-code', 'attacker-state')
    ).rejects.toThrow();

    expect(fetchMock).not.toHaveBeenCalled();
    expect(localStorage.getItem('edgewatcher_auth_tokens')).toBeNull();
  });

  // A callback with no state at all is the same attack without the effort.
  it('refuses a missing state', async () => {
    const { authService } = await import('@/services/auth');
    sessionStorage.setItem(STATE_KEY, 'the-state');
    sessionStorage.setItem(VERIFIER_KEY, 'the-verifier');
    const fetchMock = stubTokenEndpoint();

    await expect(authService.handleAuthCallback('some-code', null)).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  // Nothing stored: the login was started in another tab, or the tab was
  // restored. There is no verifier to complete the exchange with, so this
  // has to fail loudly rather than send a request Cognito will reject.
  it('refuses when nothing was stored for this tab', async () => {
    const { authService } = await import('@/services/auth');
    const fetchMock = stubTokenEndpoint();

    await expect(authService.handleAuthCallback('some-code', 'any-state')).rejects.toThrow();
    expect(fetchMock).not.toHaveBeenCalled();
  });

  // Single use. Left in place, a reload would re-arm the same verification
  // material for a code that has already been spent.
  it('clears the stored state and verifier, on success and on refusal alike', async () => {
    const { authService } = await import('@/services/auth');

    sessionStorage.setItem(STATE_KEY, 'the-state');
    sessionStorage.setItem(VERIFIER_KEY, 'the-verifier');
    stubTokenEndpoint();
    await authService.handleAuthCallback('auth-code', 'the-state');
    expect(sessionStorage.getItem(STATE_KEY)).toBeNull();
    expect(sessionStorage.getItem(VERIFIER_KEY)).toBeNull();

    sessionStorage.setItem(STATE_KEY, 'the-state');
    sessionStorage.setItem(VERIFIER_KEY, 'the-verifier');
    await authService.handleAuthCallback('auth-code', 'wrong').catch(() => {});
    expect(sessionStorage.getItem(STATE_KEY)).toBeNull();
    expect(sessionStorage.getItem(VERIFIER_KEY)).toBeNull();
  });
});
