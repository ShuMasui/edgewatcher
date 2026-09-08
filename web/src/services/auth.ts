import { env } from '../config/env';
import { AuthTokens, AuthUser } from '../types/auth';
import { QueryClient } from '@tanstack/react-query';

const AUTH_USER_KEY = 'edgewatcher_auth_user';
const AUTH_TOKENS_KEY = 'edgewatcher_auth_tokens';

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

  public login(): void {
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
      const authUrl = new URL(`${env.cognitoDomain}/oauth2/authorize`);
      authUrl.searchParams.set('client_id', env.cognitoClientId);
      authUrl.searchParams.set('response_type', 'code');
      authUrl.searchParams.set('scope', 'openid email profile');
      authUrl.searchParams.set('redirect_uri', env.redirectUri);
      authUrl.searchParams.set('identity_provider', 'Google');
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

  public async handleAuthCallback(code: string): Promise<void> {
    if (env.useMock) {
      return;
    }

    // Exchange authorization code for tokens
    const tokenUrl = `${env.cognitoDomain}/oauth2/token`;
    const body = new URLSearchParams({
      grant_type: 'authorization_code',
      client_id: env.cognitoClientId,
      code,
      redirect_uri: env.redirectUri,
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
