import { env } from '../config/env';
import { AuthTokens, AuthUser } from '../types/auth';
import { QueryClient } from '@tanstack/react-query';

const AUTH_USER_KEY = 'edgewatcher_auth_user';
const AUTH_TOKENS_KEY = 'edgewatcher_auth_tokens';

// The CSRF token and the PKCE verifier live in sessionStorage, not
// localStorage: sessionStorage is scoped to this tab, so another tab — or
// another page of the same origin opened later — cannot read the material
// that authorises this login. It survives the redirect out to Cognito and
// Google and back, because that is a navigation of this same tab.
//
// The cost is that a callback landing in a DIFFERENT tab finds nothing.
// That case is detected and reported rather than papered over; see
// consumeAuthRequest.
const OAUTH_STATE_KEY = 'edgewatcher_oauth_state';
const PKCE_VERIFIER_KEY = 'edgewatcher_pkce_verifier';

/** 256 bits of randomness, base64url — 43 characters, inside RFC 7636's 43-128. */
function randomUrlSafeToken(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  return base64UrlEncode(bytes);
}

function base64UrlEncode(bytes: Uint8Array): string {
  return btoa(String.fromCharCode(...bytes))
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

/** S256: the challenge is the SHA-256 of the verifier, never the verifier itself. */
async function deriveCodeChallenge(verifier: string): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(verifier));
  return base64UrlEncode(new Uint8Array(digest));
}

interface AuthRequest {
  state: string;
  verifier: string;
}

/**
 * Takes the pending login out of sessionStorage.
 *
 * It removes before it returns, unconditionally. Leaving the values behind
 * would let a reload re-arm the same verification material against a code
 * that has already been spent, which turns a single-use exchange into a
 * replayable one.
 */
function consumeAuthRequest(): AuthRequest | null {
  const state = sessionStorage.getItem(OAUTH_STATE_KEY);
  const verifier = sessionStorage.getItem(PKCE_VERIFIER_KEY);
  sessionStorage.removeItem(OAUTH_STATE_KEY);
  sessionStorage.removeItem(PKCE_VERIFIER_KEY);
  if (!state || !verifier) return null;
  return { state, verifier };
}

const MOCK_USER: AuthUser = {
  sub: 'mock-user-owner-001',
  email: 'pentyan0303@gmail.com',
  name: 'Shu Masui',
};

export class AuthService {
  private user: AuthUser | null = null;
  private tokens: AuthTokens | null = null;

  constructor() {
    this.loadFromStorage();
  }

  private loadFromStorage() {
    const userJson = localStorage.getItem(AUTH_USER_KEY);
    const tokensJson = localStorage.getItem(AUTH_TOKENS_KEY);
    if (userJson) {
      try {
        this.user = JSON.parse(userJson);
      } catch {
        this.user = null;
      }
    }
    if (tokensJson) {
      try {
        this.tokens = JSON.parse(tokensJson);
      } catch {
        this.tokens = null;
      }
    }
  }

  private saveToStorage(user: AuthUser, tokens: AuthTokens) {
    this.user = user;
    this.tokens = tokens;
    localStorage.setItem(AUTH_USER_KEY, JSON.stringify(user));
    localStorage.setItem(AUTH_TOKENS_KEY, JSON.stringify(tokens));
  }

  public isAuthenticated(): boolean {
    if (!this.user || !this.tokens) return false;
    // Check token expiration if present
    if (this.tokens.expiresAt && Date.now() > this.tokens.expiresAt) {
      return false;
    }
    return true;
  }

  public getUser(): AuthUser | null {
    return this.user;
  }

  public getAccessToken(): string | null {
    return this.tokens?.accessToken || null;
  }

  public getIdToken(): string | null {
    return this.tokens?.idToken || null;
  }

  public async login(): Promise<void> {
    if (env.useMock) {
      // Mock login immediate success
      const tokens: AuthTokens = {
        accessToken: 'mock-access-token-' + Date.now(),
        idToken: 'mock-id-token-' + Date.now(),
        expiresAt: Date.now() + 24 * 60 * 60 * 1000,
      };
      this.saveToStorage(MOCK_USER, tokens);
      window.location.href = '/';
      return;
    }

    // Cognito Hosted UI redirect with Google IdP
    if (env.cognitoDomain && env.cognitoClientId) {
      // Both are minted fresh per attempt. Reusing either across attempts
      // would let an old, possibly observed value authorise a new login.
      const state = randomUrlSafeToken();
      const verifier = randomUrlSafeToken();
      sessionStorage.setItem(OAUTH_STATE_KEY, state);
      sessionStorage.setItem(PKCE_VERIFIER_KEY, verifier);

      const authUrl = new URL(`${env.cognitoDomain}/oauth2/authorize`);
      authUrl.searchParams.set('client_id', env.cognitoClientId);
      authUrl.searchParams.set('response_type', 'code');
      authUrl.searchParams.set('scope', 'openid email profile');
      authUrl.searchParams.set('redirect_uri', env.redirectUri);
      authUrl.searchParams.set('identity_provider', 'Google');

      // Cognito echoes state back untouched; it never inspects it. The
      // check is entirely ours, which is what makes it a CSRF defence
      // rather than something the identity provider does for us.
      authUrl.searchParams.set('state', state);

      // PKCE. This app client has no secret (generate_secret = false in
      // infra/modules/cognito-web), so an authorization code on its own is
      // enough to obtain tokens. The challenge is what makes a leaked code
      // useless to anyone who does not also hold the verifier.
      authUrl.searchParams.set('code_challenge', await deriveCodeChallenge(verifier));
      authUrl.searchParams.set('code_challenge_method', 'S256');

      window.location.href = authUrl.toString();
    } else {
      console.warn('Cognito config is missing; falling back to mock login.');
      const tokens: AuthTokens = {
        accessToken: 'mock-access-token-' + Date.now(),
        idToken: 'mock-id-token-' + Date.now(),
        expiresAt: Date.now() + 24 * 60 * 60 * 1000,
      };
      this.saveToStorage(MOCK_USER, tokens);
      window.location.href = '/';
    }
  }

  /**
   * Completes the Hosted UI round trip.
   *
   * returnedState is what came back on the redirect. It is checked BEFORE
   * the code is sent anywhere: an attacker who gets this browser to land on
   * the redirect URI carrying their own authorization code would otherwise
   * have it exchanged, signing this user into the attacker's account
   * without either of them noticing.
   */
  public async handleAuthCallback(code: string, returnedState: string | null): Promise<void> {
    if (env.useMock) {
      return;
    }

    const pending = consumeAuthRequest();
    if (!pending) {
      // Nothing was issued from this tab: the login began elsewhere, or the
      // tab was restored. There is no verifier to complete PKCE with, so
      // failing here is both the safe answer and the only possible one.
      throw new Error('auth: no pending authorization request in this tab');
    }
    if (!returnedState || returnedState !== pending.state) {
      // Plain comparison is right: state is not a server-side secret, it is
      // a value this browser generated and kept in its own sessionStorage.
      // There is no timing channel to defend.
      throw new Error('auth: state mismatch on the OAuth callback');
    }

    // Exchange authorization code for tokens
    const tokenUrl = `${env.cognitoDomain}/oauth2/token`;
    const body = new URLSearchParams({
      grant_type: 'authorization_code',
      client_id: env.cognitoClientId,
      code,
      redirect_uri: env.redirectUri,
      code_verifier: pending.verifier,
    });

    const response = await fetch(tokenUrl, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/x-www-form-urlencoded',
      },
      body: body.toString(),
    });

    if (!response.ok) {
      throw new Error(`Token exchange failed: ${response.statusText}`);
    }

    const data = await response.json();
    const idToken = data.id_token;
    const accessToken = data.access_token;
    const expiresIn = data.expires_in || 3600;

    // Decode JWT payload (without external dep)
    const base64Url = idToken.split('.')[1];
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/');
    const jsonPayload = decodeURIComponent(
      atob(base64)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join('')
    );
    const payload = JSON.parse(jsonPayload);

    const user: AuthUser = {
      sub: payload.sub,
      email: payload.email || '',
      name: payload.name,
      picture: payload.picture,
    };

    const tokens: AuthTokens = {
      accessToken,
      idToken,
      refreshToken: data.refresh_token,
      expiresAt: Date.now() + expiresIn * 1000,
    };

    this.saveToStorage(user, tokens);
  }

  /**
   * Log out flow according to docs/03-web.md §1.4.2:
   * 1. TanStack Query cache is cleared
   * 2. Local tokens are cleared
   * 3. Redirects to Cognito Hosted UI /logout?client_id=...&logout_uri=...
   */
  public logout(queryClient?: QueryClient): void {
    // 1. Clear query cache
    if (queryClient) {
      queryClient.clear();
    }

    // 2. Clear local storage
    this.user = null;
    this.tokens = null;
    localStorage.removeItem(AUTH_USER_KEY);
    localStorage.removeItem(AUTH_TOKENS_KEY);

    // 3. Redirect to Cognito logout or /login
    if (env.useMock || !env.cognitoDomain || !env.cognitoClientId) {
      window.location.href = '/login';
      return;
    }

    const logoutUrl = new URL(`${env.cognitoDomain}/logout`);
    logoutUrl.searchParams.set('client_id', env.cognitoClientId);
    logoutUrl.searchParams.set('logout_uri', env.logoutUri);
    window.location.href = logoutUrl.toString();
  }
}

export const authService = new AuthService();
