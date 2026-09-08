import { useQuery, useQueryClient } from '@tanstack/react-query';
import { apiClient } from '../services/api-client';

export const OBSERVATIONS_QUERY_KEY = (deviceId: string, dateStr: string) => ['observations', deviceId, dateStr];
// deviceId もキーに含める。同じ observationId が別の端末に存在すること自体は
// ULID なので起きないが、キャッシュのキーは API の引数と一致していなければ
// ならない — 片方だけ変えたときに古いエントリが返る。
export const OBSERVATION_IMAGE_QUERY_KEY = (observationId: string, deviceId: string) => [
  'observation-image',
  deviceId,
  observationId,
];

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

export function useObservationImage(observationId: string | null, deviceId: string | null) {
  const queryClient = useQueryClient();

  const query = useQuery({
    queryKey: OBSERVATION_IMAGE_QUERY_KEY(observationId || '', deviceId || ''),
    queryFn: () => apiClient.getObservationImage(observationId!, deviceId!),
    enabled: !!observationId && !!deviceId,
    staleTime: 10 * 60 * 1000, // 10 minutes (signed URLs expire in 15 minutes)
    retry: 1,
  });

  const handleImageError = () => {
    if (observationId && deviceId) {
      queryClient.invalidateQueries({
        queryKey: OBSERVATION_IMAGE_QUERY_KEY(observationId, deviceId),
      });
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
