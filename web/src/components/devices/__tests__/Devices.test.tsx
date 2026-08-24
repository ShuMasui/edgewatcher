import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AuthProvider } from '@/hooks/use-auth';
import { DevicesPage } from '@/routes/DevicesPage';
import { authService } from '@/services/auth';
import { apiClient } from '@/services/api-client';
import { Device, AppConfig } from '@/types/domain';

const createTestQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });

describe('DevicesPage', () => {
  const mockConfig: AppConfig = {
    retentionDays: 7,
    deviceLimit: 3,
    intervalOptions: [5, 10, 15],
  };

  const mockDevices: Device[] = [
    {
      deviceId: 'd1',
      ownerId: 'mock-user',
      name: '玄関',
      status: 'PAIRED',
      interval: 5,
      lastReceivedAt: new Date().toISOString(),
    },
    {
      deviceId: 'd2',
      ownerId: 'mock-user',
      name: '物置',
      status: 'PENDING',
      interval: 5,
    },
    {
      deviceId: 'd3',
      ownerId: 'mock-user',
      name: '旧・玄関',
      status: 'DISCONNECTED',
      interval: 10,
    },
  ];

  beforeEach(() => {
    vi.restoreAllMocks();
    vi.spyOn(authService, 'isAuthenticated').mockReturnValue(true);
    vi.spyOn(authService, 'getUser').mockReturnValue({
      sub: 'mock-user',
      email: 'pentyan0303@gmail.com',
      name: 'Shu',
    });
    vi.spyOn(apiClient, 'getAppConfig').mockResolvedValue(mockConfig);
    vi.spyOn(apiClient, 'getDevices').mockResolvedValue(mockDevices);
  });

  it('renders table with correct status buttons and count', async () => {
    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AuthProvider>
            <DevicesPage />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByTestId('device-count-display')).toHaveTextContent('3 / 3 台');
    expect(screen.getByTestId('action-disconnect-d1')).toHaveTextContent('セッション切断');
    expect(screen.getByTestId('action-qr-d2')).toHaveTextContent('QR を表示');
    expect(screen.getByTestId('action-repair-d3')).toHaveTextContent('再ペアリング');
  });

  it('shows limit banner and disables "端末を追加" button when device limit is reached', async () => {
    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AuthProvider>
            <DevicesPage />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByTestId('device-limit-banner')).toBeInTheDocument();
    expect(screen.getByText(/端末の上限\(3台\)に達しています/)).toBeInTheDocument();
    expect(screen.getByTestId('add-device-btn')).toBeDisabled();
  });

  it('handles interval change successfully', async () => {
    const updateSpy = vi.spyOn(apiClient, 'updateDevice').mockResolvedValue({
      ...mockDevices[0],
      interval: 10,
    });

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AuthProvider>
            <DevicesPage />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    const select = await screen.findByTestId('interval-select-d1');
    expect(select).toHaveValue('5');

    fireEvent.change(select, { target: { value: '10' } });

    await waitFor(() => {
      expect(updateSpy).toHaveBeenCalledWith('d1', { interval: 10 });
    });
  });

  it('opens delete confirmation modal and warns about observation deletion', async () => {
    const deleteSpy = vi.spyOn(apiClient, 'deleteDevice').mockResolvedValue({ success: true });

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <AuthProvider>
            <DevicesPage />
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    // Click kebab menu for first device
    const kebabButtons = await screen.findAllByTestId('kebab-button');
    fireEvent.click(kebabButtons[0]);

    const deleteMenuItem = screen.getByTestId('menu-delete-btn');
    fireEvent.click(deleteMenuItem);

    expect(screen.getByText('「玄関」を削除しますか?')).toBeInTheDocument();
    expect(screen.getByText(/この端末の観測画像もすべて削除されます/)).toBeInTheDocument();

    const confirmBtn = screen.getByTestId('confirm-delete-btn');
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(deleteSpy).toHaveBeenCalledWith('d1');
    });
  });
});
