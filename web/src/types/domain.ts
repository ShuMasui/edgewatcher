/**
 * Device status in DynamoDB (Raw status)
 */
export type DeviceRawStatus = 'PENDING' | 'PAIRED' | 'DISCONNECTED' | 'ARCHIVED';

/**
 * Derived display status on Web UI
 */
export type DeviceDerivedStatus = 'pending' | 'paired' | 'stale' | 'disconnected';

export interface PairingSession {
  pairingCode: string;
  expiresAt: number; // epoch seconds
  status: 'PENDING' | 'CONSUMED';
  createdAt?: string;
  consumedAt?: string;
}

export interface Device {
  deviceId: string;
  ownerId: string;
  name: string;
  status: DeviceRawStatus;
  interval: number; // 5 | 10 | 15 minutes
  lastReceivedAt?: string; // ISO 8601
  latestThumbnailUrl?: string; // Signed S3 URL
  latestCapturedAt?: string; // ISO 8601
  createdAt?: string;
  archivedAt?: string;
  activePairingSession?: PairingSession | null;
}

export interface Observation {
  observationId: string;
  deviceId: string;
  capturedAt: string; // ISO 8601
  thumbnailUrl: string; // Signed S3 URL
  imageUrl?: string; // Signed S3 URL (when fetched on-demand)
  lat?: number;
  lng?: number;
  expiresAt?: number;
}

export interface AppConfig {
  retentionDays: number;
  deviceLimit: number;
  intervalOptions: number[];
}
