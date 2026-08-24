import { useQuery } from '@tanstack/react-query';
import { apiClient } from '../services/api-client';
import { AppConfig } from '../types/domain';

export const APP_CONFIG_QUERY_KEY = ['app-config'];

const DEFAULT_CONFIG: AppConfig = {
  retentionDays: 7,
  deviceLimit: 10,
  intervalOptions: [5, 10, 15],
};

export function useAppConfig() {
  const query = useQuery({
    queryKey: APP_CONFIG_QUERY_KEY,
    queryFn: () => apiClient.getAppConfig(),
    staleTime: Infinity, // 03-web.md §3.8: Fetched once at startup, never stale during session
    gcTime: Infinity,
    placeholderData: DEFAULT_CONFIG,
  });

  return {
    config: query.data || DEFAULT_CONFIG,
    isLoading: query.isLoading,
    error: query.error,
    refetch: query.refetch,
  };
}
