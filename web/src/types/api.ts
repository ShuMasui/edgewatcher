import { AppConfig, Device, Observation, PairingSession } from './domain';

export interface ApiErrorPayload {
  error: {
    code: string;
    message: string;
  };
}

export class ApiError extends Error {
  statusCode: number;
  code: string;

  constructor(statusCode: number, code: string, message: string) {
    super(message);
    this.name = 'ApiError';
    this.statusCode = statusCode;
    this.code = code;
  }
}

export type GetAppConfigResponse = AppConfig;

export type GetDevicesResponse = Device[];

export interface CreateDeviceRequest {
  name: string;
}

export interface CreateDeviceResponse {
  device: Device;
  pairingSession: PairingSession;
}

export interface CreatePairingSessionResponse {
  pairingCode: string;
  expiresAt: number;
  status: 'PENDING' | 'CONSUMED';
}

export interface GetLatestPairingSessionResponse {
  pairingCode: string;
  expiresAt: number;
  status: 'PENDING' | 'CONSUMED';
}

export interface DisconnectDeviceResponse {
  deviceId: string;
  status: 'DISCONNECTED';
}

export interface UpdateDeviceRequest {
  name?: string;
  interval?: number;
}

export type UpdateDeviceResponse = Device;

export interface DeleteDeviceResponse {
  success: boolean;
}

export type GetObservationsResponse = Observation[];

export interface GetObservationImageResponse {
  observationId: string;
  imageUrl: string;
  expiresAt: number;
}
