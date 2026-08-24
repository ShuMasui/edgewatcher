export interface EnvConfig {
  apiBaseUrl: string;
  cognitoDomain: string;
  cognitoClientId: string;
  redirectUri: string;
  logoutUri: string;
  useMock: boolean;
}

export const env: EnvConfig = {
  apiBaseUrl: import.meta.env.VITE_API_BASE_URL || '',
  cognitoDomain: import.meta.env.VITE_COGNITO_DOMAIN || '',
  cognitoClientId: import.meta.env.VITE_COGNITO_CLIENT_ID || '',
  redirectUri: import.meta.env.VITE_REDIRECT_URI || `${window.location.origin}/`,
  logoutUri: import.meta.env.VITE_LOGOUT_URI || `${window.location.origin}/login`,
  // If apiBaseUrl is not specified or VITE_USE_MOCK is explicitly true, enable mock mode
  useMock:
    import.meta.env.VITE_USE_MOCK === 'true' ||
    !import.meta.env.VITE_API_BASE_URL ||
    import.meta.env.VITE_API_BASE_URL.trim() === '',
};
