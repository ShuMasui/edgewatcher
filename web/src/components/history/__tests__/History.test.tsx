import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { AuthProvider } from '@/hooks/use-auth';
import { HistoryPage } from '@/routes/HistoryPage';
import { authService } from '@/services/auth';
import { apiClient } from '@/services/api-client';
import { Device, Observation, AppConfig } from '@/types/domain';

const createTestQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
    },
  });

describe('HistoryPage', () => {
  const mockConfig: AppConfig = {
    retentionDays: 1, // Dev environment: 1 day only
    deviceLimit: 10,
    intervalOptions: [5, 10, 15],
  };

  const mockDevice: Device = {
    deviceId: 'dev-001',
    ownerId: 'mock-user',
    name: '玄関',
    status: 'PAIRED',
    interval: 5,
    lastReceivedAt: new Date().toISOString(),
  };

  const mockObservations: Observation[] = [
    {
      observationId: 'obs-1',
      deviceId: 'dev-001',
      capturedAt: '2026-08-24T10:00:00Z',
      thumbnailUrl: 'data:image/svg+xml;utf8,<svg id="thumb1"></svg>',
      imageUrl: 'data:image/svg+xml;utf8,<svg id="img1"></svg>',
    },
    {
      observationId: 'obs-2',
      deviceId: 'dev-001',
      capturedAt: '2026-08-24T10:05:00Z',
      thumbnailUrl: 'data:image/svg+xml;utf8,<svg id="thumb2"></svg>',
      imageUrl: 'data:image/svg+xml;utf8,<svg id="img2"></svg>',
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
    vi.spyOn(apiClient, 'getDevices').mockResolvedValue([mockDevice]);
    vi.spyOn(apiClient, 'getObservations').mockResolvedValue(mockObservations);
  });

  it('renders history viewer with main image and scrubber', async () => {
    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/devices/dev-001/history']}>
          <AuthProvider>
            <Routes>
              <Route path="/devices/:deviceId/history" element={<HistoryPage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByText('玄関')).toBeInTheDocument();
    expect(screen.getByTestId('main-history-image')).toBeInTheDocument();
    expect(screen.getByTestId('scrubber')).toBeInTheDocument();
    expect(screen.getByTestId('thumbnail-strip')).toBeInTheDocument();
  });

  // 実 API の一覧は imageUrl を返さない(05-backend.md §1.5: 1日分すべてに
  // 本画像の署名を付けると、開きもしない画像の有効な認可をブラウザに
  // 大量に渡すことになる)。モックは一覧に imageUrl を載せるため、上の
  // テストだけでは実 API に繋いだ瞬間に「サムネイルを引き伸ばして表示し、
  // 本画像は一度も取りに行かない」状態になったことに気づけない。
  it('fetches the full-size image on demand when the list omits imageUrl', async () => {
    const listWithoutFullImages: Observation[] = mockObservations.map(
      ({ imageUrl: _dropped, ...rest }) => rest
    );
    vi.spyOn(apiClient, 'getObservations').mockResolvedValue(listWithoutFullImages);
    const getImage = vi.spyOn(apiClient, 'getObservationImage').mockResolvedValue({
      observationId: 'obs-2',
      imageUrl: 'data:image/svg+xml;utf8,<svg id="full2"></svg>',
      expiresAt: Math.floor(Date.now() / 1000) + 900,
    });

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/devices/dev-001/history']}>
          <AuthProvider>
            <Routes>
              <Route path="/devices/:deviceId/history" element={<HistoryPage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    const image = await screen.findByTestId('main-history-image');
    await waitFor(() => {
      expect(image).toHaveAttribute('src', 'data:image/svg+xml;utf8,<svg id="full2"></svg>');
    });

    // deviceId を伴って呼ばれること。省くとサーバは 400 を返し、観測が
    // どの端末のものかを検証する経路そのものが成立しない(G11)。
    expect(getImage).toHaveBeenCalledWith('obs-2', 'dev-001');
  });

  it('disables previous date arrow and shows retention note when retentionDays is 1 (today only)', async () => {
    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/devices/dev-001/history']}>
          <AuthProvider>
            <Routes>
              <Route path="/devices/:deviceId/history" element={<HistoryPage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    const prevBtn = await screen.findByTestId('prev-date-btn');
    expect(prevBtn).toBeDisabled();
    expect(screen.getByTestId('retention-note')).toHaveTextContent('これより前の画像は保持期間(1日)を過ぎています');
  });

  it('displays empty day placeholder when 0 observations are returned', async () => {
    vi.spyOn(apiClient, 'getObservations').mockResolvedValue([]);

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/devices/dev-001/history']}>
          <AuthProvider>
            <Routes>
              <Route path="/devices/:deviceId/history" element={<HistoryPage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByTestId('empty-day-placeholder')).toHaveTextContent('この日、この端末からは画像が届いていません');
    expect(screen.getByTestId('empty-strip')).toHaveTextContent('表示できる画像がありません');
  });

  it('displays not found error screen when device does not exist', async () => {
    vi.spyOn(apiClient, 'getDevices').mockResolvedValue([]);

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/devices/non-existent-id/history']}>
          <AuthProvider>
            <Routes>
              <Route path="/devices/:deviceId/history" element={<HistoryPage />} />
            </Routes>
          </AuthProvider>
        </MemoryRouter>
      </QueryClientProvider>
    );

    expect(await screen.findByText('この端末は見つかりません')).toBeInTheDocument();
    expect(screen.getByTestId('back-to-devices-btn')).toBeInTheDocument();
  });
});
