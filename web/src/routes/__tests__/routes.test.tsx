import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AuthProvider } from '@/hooks/use-auth';
import { AppRoutes } from '../index';
import { authService } from '@/services/auth';

const createTestQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  });

describe('AppRoutes and Auth flow', () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it('redirects unauthenticated user from / to /login with redirect param', () => {
    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/']}>
          <AuthProvider>
            <AppRoutes />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(screen.getByTestId('login-page')).toBeInTheDocument();
    expect(screen.getByText('Google で続行')).toBeInTheDocument();
  });

  it('allows authenticated user to view dashboard at /', async () => {
    // Authenticate mock user
    vi.spyOn(authService, 'isAuthenticated').mockReturnValue(true);
    vi.spyOn(authService, 'getUser').mockReturnValue({
      sub: 'mock-sub',
      email: 'test@example.com',
      name: 'Test User',
    });

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/']}>
          <AuthProvider>
            <AppRoutes />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByTestId('dashboard-topbar')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: '最新画像' })).toBeInTheDocument();
    expect(screen.getByTestId('nav-dashboard')).toBeInTheDocument();
  });

  it('renders 404 page for unmatched routes', () => {
    vi.spyOn(authService, 'isAuthenticated').mockReturnValue(true);
    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/some/unknown/path']}>
          <AuthProvider>
            <AppRoutes />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(screen.getByText('ページが見つかりませんでした')).toBeInTheDocument();
    expect(screen.getByTestId('not-found-home-btn')).toBeInTheDocument();
  });
});
