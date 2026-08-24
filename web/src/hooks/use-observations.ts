import { useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../services/api-client';

export const OBSERVATIONS_QUERY_KEY = (deviceId: string, dateStr: string) => ['observations', deviceId, dateStr];
export const OBSERVATION_IMAGE_QUERY_KEY = (observationId: string) => ['observation-image', observationId];

export function useObservations(deviceId: string, dateStr: string) {
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: OBSERVATIONS_QUERY_KEY(deviceId, dateStr),
    queryFn: () => apiClient.getObservations(deviceId, dateStr),
    enabled: !!deviceId && !!dateStr,
    staleTime: 60000,
    retry: 2,
  });

  // 1.10.5: S3 signed URL auto-recovery on image error
  const handleImageError = () => {
    queryClient.invalidateQueries({ queryKey: OBSERVATIONS_QUERY_KEY(deviceId, dateStr) });
  };

  return {
    observations: query.data || [],
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    refetch: query.refetch,
    handleImageError,
  };
}

export function useObservationImage(observationId: string | null) {
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: OBSERVATION_IMAGE_QUERY_KEY(observationId || ''),
    queryFn: () => apiClient.getObservationImage(observationId!),
    enabled: !!observationId,
    staleTime: 10 * 60 * 1000, // 10 minutes (signed URLs expire in 15 minutes)
    retry: 1,
  });

  const handleImageError = () => {
    if (observationId) {
      queryClient.invalidateQueries({ queryKey: OBSERVATION_IMAGE_QUERY_KEY(observationId) });
    }
  };

  return {
    imageData: query.data || null,
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    handleImageError,
  };
}
