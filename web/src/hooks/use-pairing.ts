import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../services/api-client';
import { DEVICES_QUERY_KEY } from './use-devices';

export function useCreateDevice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (name: string) => apiClient.createDevice({ name }),
    retry: false,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: DEVICES_QUERY_KEY });
    },
  });
}

export function useCreatePairingSession() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (deviceId: string) => apiClient.createPairingSession(deviceId),
    retry: false,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: DEVICES_QUERY_KEY });
    },
  });
}

export function usePairingSessionPolling(deviceId: string | null, enabled: boolean) {
  const queryClient = useQueryClient();

  return useQuery({
    queryKey: ['pairing-session-latest', deviceId],
    queryFn: async () => {
      if (!deviceId) return null;
      const session = await apiClient.getLatestPairingSession(deviceId);
      if (session?.status === 'CONSUMED') {
        // Pairing consumed! Invalidate device list to reflect PAIRED state
        queryClient.invalidateQueries({ queryKey: DEVICES_QUERY_KEY });
      }
      return session;
    },
    enabled: !!deviceId && enabled,
    // 03-web.md §1.8.1: Poll every 2 seconds
    refetchInterval: 2000,
    retry: false,
  });
}
