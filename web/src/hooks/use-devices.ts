import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../services/api-client';
import { Device } from '../types/domain';
import { useState } from 'react';

export const DEVICES_QUERY_KEY = ['devices'];

export function useDevices() {
  const [lastSuccessTime, setLastSuccessTime] = useState<Date | null>(null);

  const query = useQuery({
    queryKey: DEVICES_QUERY_KEY,
    queryFn: async () => {
      const data = await apiClient.getDevices();
      setLastSuccessTime(new Date());
      return data;
    },
    // 03-web.md §1.5: 60s polling interval
    refetchInterval: 60000,
    staleTime: 30000,
  });

  // Track if polling fails after having data (stale error state)
  const isPollingError = query.isError && !!query.data;

  return {
    devices: query.data || [],
    isLoading: query.isLoading,
    isInitialError: query.isError && !query.data,
    isPollingError,
    error: query.error,
    lastSuccessTime,
    refetch: query.refetch,
  };
}

export function useDevice(deviceId: string) {
  const { devices, isLoading, error, refetch } = useDevices();
  const device = devices.find((d) => d.deviceId === deviceId) || null;

  return {
    device,
    isLoading,
    isNotFound: !isLoading && !device,
    error,
    refetch,
  };
}

export function useUpdateDevice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ deviceId, data }: { deviceId: string; data: { name?: string; interval?: number } }) =>
      apiClient.updateDevice(deviceId, data),
    // 03-web.md §1.10.4: Mutations are not automatically retried
    retry: false,
    onSuccess: (updatedDevice: Device) => {
      queryClient.setQueryData<Device[]>(DEVICES_QUERY_KEY, (old) => {
        if (!old) return [updatedDevice];
        return old.map((d) => (d.deviceId === updatedDevice.deviceId ? updatedDevice : d));
      });
      queryClient.invalidateQueries({ queryKey: DEVICES_QUERY_KEY });
    },
  });
}

export function useDisconnectDevice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (deviceId: string) => apiClient.disconnectDevice(deviceId),
    retry: false,
    onSuccess: (_, deviceId) => {
      queryClient.setQueryData<Device[]>(DEVICES_QUERY_KEY, (old) => {
        if (!old) return [];
        return old.map((d) => (d.deviceId === deviceId ? { ...d, status: 'DISCONNECTED', activePairingSession: null } : d));
      });
      queryClient.invalidateQueries({ queryKey: DEVICES_QUERY_KEY });
    },
  });
}

export function useDeleteDevice() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (deviceId: string) => apiClient.deleteDevice(deviceId),
    retry: false,
    onSuccess: (_, deviceId) => {
      queryClient.setQueryData<Device[]>(DEVICES_QUERY_KEY, (old) => {
        if (!old) return [];
        return old.filter((d) => d.deviceId !== deviceId);
      });
      queryClient.invalidateQueries({ queryKey: DEVICES_QUERY_KEY });
    },
  });
}
