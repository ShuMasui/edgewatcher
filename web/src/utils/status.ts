import { Device, DeviceDerivedStatus } from '../types/domain';

/**
 * Derives UI status from device raw status and lastReceivedAt timestamp.
 * Status logic per docs/03-web.md §1.3:
 * - PENDING or active pairing session -> 'pending' (ペアリング待ち)
 * - PAIRED and within threshold -> 'paired' (接続中)
 * - PAIRED and beyond threshold -> 'stale' (応答なし)
 * - DISCONNECTED -> 'disconnected' (切断済み)
 */
export function getDerivedDeviceStatus(device: Device, now = new Date()): DeviceDerivedStatus {
  if (device.status === 'PENDING' || device.activePairingSession?.status === 'PENDING') {
    return 'pending';
  }

  if (device.status === 'DISCONNECTED') {
    return 'disconnected';
  }

  if (device.status === 'PAIRED') {
    if (!device.lastReceivedAt) {
      return 'stale';
    }

    const lastReceived = new Date(device.lastReceivedAt).getTime();
    if (isNaN(lastReceived)) {
      return 'stale';
    }

    // Threshold = transmission interval * 3 minutes (03-web.md §1.3 & AUTH-09)
    const thresholdMinutes = (device.interval || 5) * 3;
    const thresholdMs = thresholdMinutes * 60 * 1000;
    const elapsed = now.getTime() - lastReceived;

    return elapsed <= thresholdMs ? 'paired' : 'stale';
  }

  return 'disconnected';
}

export function getStatusBadgeInfo(status: DeviceDerivedStatus): {
  label: string;
  dotClass: string;
} {
  switch (status) {
    case 'paired':
      return { label: '接続中', dotClass: 'ok' };
    case 'stale':
      return { label: '応答なし', dotClass: 'warn' };
    case 'pending':
      return { label: 'ペアリング待ち', dotClass: 'pending' };
    case 'disconnected':
      return { label: '切断済み', dotClass: 'disconnected' };
  }
}
