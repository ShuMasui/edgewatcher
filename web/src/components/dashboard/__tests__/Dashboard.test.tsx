import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AuthProvider } from '@/hooks/use-auth';
import { DashboardPage } from '@/routes/DashboardPage';
import { authService } from '@/services/auth';
import { apiClient } from '@/services/api-client';
import { Device } from '@/types/domain';

const createTestQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  });

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(authService, 'isAuthenticated').mockReturnValue(true);
    vi.spyOn(authService, 'getUser').mockReturnValue({
      sub: 'mock-user',
      email: 'pentyan0303@gmail.com',
      name: 'Shu',
    });
  });

  it('renders device cards with correct names and status badges', async () => {
    const mockDevices: Device[] = [
      {
        deviceId: 'd1',
        ownerId: 'mock-user',
        name: '玄関',
        status: 'PAIRED',
        interval: 5,
        lastReceivedAt: new Date().toISOString(),
        latestThumbnailUrl: 'data:image/svg+xml;utf8,<svg></svg>',
      },
      {
        deviceId: 'd2',
        ownerId: 'mock-user',
        name: '物置',
        status: 'PENDING',
        interval: 5,
      },
    ];

    vi.spyOn(apiClient, 'getDevices').mockResolvedValue(mockDevices);

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AuthProvider>
            <DashboardPage />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByText('玄関')).toBeInTheDocument();
    expect(screen.getByText('物置')).toBeInTheDocument();
    expect(screen.getByText('接続中')).toBeInTheDocument();
    expect(screen.getByText('ペアリング待ち')).toBeInTheDocument();
    expect(screen.getByText('QR コードの読み取りを待っています')).toBeInTheDocument();
    expect(screen.getByTestId('device-count')).toHaveTextContent('2 台');
  });

  it('renders empty dashboard state when 0 devices are returned', async () => {
    vi.spyOn(apiClient, 'getDevices').mockResolvedValue([]);

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AuthProvider>
            <DashboardPage />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByTestId('empty-dashboard')).toBeInTheDocument();
    expect(screen.getByText('まだ端末がありません')).toBeInTheDocument();
    expect(screen.getByTestId('empty-add-device-btn')).toBeInTheDocument();
  });

  it('renders inline error when initial fetch fails', async () => {
    vi.spyOn(apiClient, 'getDevices').mockRejectedValue(new Error('Network error'));

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AuthProvider>
            <DashboardPage />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByTestId('inline-error')).toBeInTheDocument();
    expect(screen.getByText('読み込めませんでした')).toBeInTheDocument();
  });
});
