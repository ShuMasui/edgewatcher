import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { PairingModal } from '../PairingModal';
import { apiClient } from '@/services/api-client';
import { ApiError } from '@/types/api';

const createTestQueryClient = () =>
  new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: 0 },
      mutations: { retry: false },
    },
  });

describe('PairingModal', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('progresses from name input to QR code display', async () => {
    const createDeviceSpy = vi.spyOn(apiClient, 'createDevice').mockResolvedValue({
      device: {
        deviceId: 'dev-new-001',
        ownerId: 'mock-user',
        name: '物置',
        status: 'PENDING',
        interval: 5,
      },
      pairingSession: {
        pairingCode: 'PAIR123456',
        expiresAt: Math.floor(Date.now() / 1000) + 300,
        status: 'PENDING',
      },
    });

    vi.spyOn(apiClient, 'getLatestPairingSession').mockResolvedValue({
      pairingCode: 'PAIR123456',
      expiresAt: Math.floor(Date.now() / 1000) + 300,
      status: 'PENDING',
    });

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <PairingModal isOpen={true} onClose={() => {}} />
      </QueryClientProvider>
    );

    expect(screen.getByTestId('pairing-name-step')).toBeInTheDocument();

    const input = screen.getByTestId('pairing-name-input');
    fireEvent.change(input, { target: { value: '物置' } });

    const submitBtn = screen.getByTestId('pairing-name-submit-btn');
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(createDeviceSpy).toHaveBeenCalledWith({ name: '物置' });
    });

    expect(await screen.findByTestId('pairing-qr-step')).toBeInTheDocument();
    expect(screen.getByText('「物置」をペアリング')).toBeInTheDocument();
    expect(screen.getByTestId('pairing-countdown')).toBeInTheDocument();
  });

  it('transitions to done step when polling detects pairing session is CONSUMED', async () => {
    vi.spyOn(apiClient, 'createDevice').mockResolvedValue({
      device: {
        deviceId: 'dev-new-001',
        ownerId: 'mock-user',
        name: '物置',
        status: 'PENDING',
        interval: 5,
      },
      pairingSession: {
        pairingCode: 'PAIR123456',
        expiresAt: Math.floor(Date.now() / 1000) + 300,
        status: 'PENDING',
      },
    });

    // Polling returns CONSUMED
    vi.spyOn(apiClient, 'getLatestPairingSession').mockResolvedValue({
      pairingCode: 'PAIR123456',
      expiresAt: Math.floor(Date.now() / 1000) + 300,
      status: 'CONSUMED',
    });

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <PairingModal isOpen={true} onClose={() => {}} />
      </QueryClientProvider>
    );

    const input = screen.getByTestId('pairing-name-input');
    fireEvent.change(input, { target: { value: '物置' } });
    fireEvent.click(screen.getByTestId('pairing-name-submit-btn'));

    expect(await screen.findByTestId('pairing-done-step')).toBeInTheDocument();
    expect(screen.getByText('「物置」が接続しました')).toBeInTheDocument();
    expect(screen.getByTestId('pairing-success-check')).toBeInTheDocument();
  });

  it('handles 429 device limit error with friendly guidance', async () => {
    vi.spyOn(apiClient, 'createDevice').mockRejectedValue(
      new ApiError(429, 'DEVICE_LIMIT_EXCEEDED', 'Device limit exceeded')
    );

    const queryClient = createTestQueryClient();

    render(
      <QueryClientProvider client={queryClient}>
        <PairingModal isOpen={true} onClose={() => {}} />
      </QueryClientProvider>
    );

    const input = screen.getByTestId('pairing-name-input');
    fireEvent.change(input, { target: { value: '物置' } });
    fireEvent.click(screen.getByTestId('pairing-name-submit-btn'));

    expect(await screen.findByTestId('pairing-429-view')).toBeInTheDocument();
    expect(screen.getByText('端末を追加できませんでした')).toBeInTheDocument();
  });
});
