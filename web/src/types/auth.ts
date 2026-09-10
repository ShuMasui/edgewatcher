export interface AuthUser {
  sub: string;
  email: string;
  name?: string;
  picture?: string;
}

export interface AuthTokens {
  accessToken: string;
  idToken: string;
  refreshToken?: string;
  expiresAt: number; // epoch milliseconds
}
