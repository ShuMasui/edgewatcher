import { env } from '../config/env';
import {
  ApiError,
  CreateDeviceRequest,
  CreateDeviceResponse,
  CreatePairingSessionResponse,
  DeleteDeviceResponse,
  DisconnectDeviceResponse,
  GetAppConfigResponse,
  GetDevicesResponse,
  GetLatestPairingSessionResponse,
  GetObservationImageResponse,
  GetObservationsResponse,
  UpdateDeviceRequest,
  UpdateDeviceResponse,
} from '../types/api';
import { authService } from './auth';
import { mockServer } from './mock-server';

class ApiClient {
  private async request<T>(path: string, options: RequestInit = {}): Promise<T> {
    const url = `${env.apiBaseUrl}${path}`;
    const token = authService.getIdToken();

    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string>),
    };

    if (token) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const response = await fetch(url, {
      ...options,
      headers,
    });

    // 1.10.1: 401 interception at HTTP client level
    if (response.status === 401) {
      authService.logout();
      const currentPath = window.location.pathname + window.location.search;
      if (window.location.pathname !== '/login') {
        window.location.href = `/login?redirect=${encodeURIComponent(currentPath)}`;
      }
      throw new ApiError(401, 'UNAUTHORIZED', '認証の有効期限が切れました。再ログインしてください。');
    }

    if (!response.ok) {
      let code = 'UNKNOWN_ERROR';
      let message = `Request failed with status ${response.status}`;
      try {
        const errorData = await response.json();
        if (errorData?.error) {
          code = errorData.error.code || code;
          message = errorData.error.message || message;
        }
      } catch {
        // body was not JSON
      }
      throw new ApiError(response.status, code, message);
    }

    if (response.status === 204) {
      return {} as T;
    }

    return response.json();
  }

  // GET /app-config
  async getAppConfig(): Promise<GetAppConfigResponse> {
    if (env.useMock) {
      return mockServer.getAppConfig();
    }
    return this.request<GetAppConfigResponse>('/app-config');
  }

  // GET /devices
  async getDevices(): Promise<GetDevicesResponse> {
    if (env.useMock) {
      return mockServer.getDevices();
    }
    return this.request<GetDevicesResponse>('/devices');
  }

  // POST /devices
  async createDevice(data: CreateDeviceRequest): Promise<CreateDeviceResponse> {
    if (env.useMock) {
      return mockServer.createDevice(data.name);
    }
    return this.request<CreateDeviceResponse>('/devices', {
      method: 'POST',
      body: JSON.stringify(data),
    });
  }

  // POST /devices/{id}/pairing-sessions
  async createPairingSession(deviceId: string): Promise<CreatePairingSessionResponse> {
    if (env.useMock) {
      return mockServer.createPairingSession(deviceId);
    }
    return this.request<CreatePairingSessionResponse>(`/devices/${deviceId}/pairing-sessions`, {
      method: 'POST',
    });
  }

  // GET /devices/{id}/pairing-sessions/latest
  async getLatestPairingSession(deviceId: string): Promise<GetLatestPairingSessionResponse | null> {
    if (env.useMock) {
      return mockServer.getLatestPairingSession(deviceId);
    }
    return this.request<GetLatestPairingSessionResponse>(`/devices/${deviceId}/pairing-sessions/latest`);
  }

  // POST /devices/{id}/disconnect
  async disconnectDevice(deviceId: string): Promise<DisconnectDeviceResponse> {
    if (env.useMock) {
      const dev = await mockServer.disconnectDevice(deviceId);
      return { deviceId: dev.deviceId, status: 'DISCONNECTED' };
    }
    return this.request<DisconnectDeviceResponse>(`/devices/${deviceId}/disconnect`, {
      method: 'POST',
    });
  }

  // PATCH /devices/{id}
  async updateDevice(deviceId: string, data: UpdateDeviceRequest): Promise<UpdateDeviceResponse> {
    if (env.useMock) {
      return mockServer.updateDevice(deviceId, data);
    }
    return this.request<UpdateDeviceResponse>(`/devices/${deviceId}`, {
      method: 'PATCH',
      body: JSON.stringify(data),
    });
  }

  // DELETE /devices/{id}
  async deleteDevice(deviceId: string): Promise<DeleteDeviceResponse> {
    if (env.useMock) {
      await mockServer.deleteDevice(deviceId);
      return { success: true };
    }
    await this.request<void>(`/devices/${deviceId}`, {
      method: 'DELETE',
    });
    return { success: true };
  }

  // GET /devices/{id}/observations?date=YYYY-MM-DD
  async getObservations(deviceId: string, date: string): Promise<GetObservationsResponse> {
    if (env.useMock) {
      return mockServer.getObservations(deviceId, date);
    }
    return this.request<GetObservationsResponse>(`/devices/${deviceId}/observations?date=${encodeURIComponent(date)}`);
  }

  // GET /observations/{id}/image
  async getObservationImage(observationId: string): Promise<GetObservationImageResponse> {
    if (env.useMock) {
      return mockServer.getObservationImage(observationId);
    }
    return this.request<GetObservationImageResponse>(`/observations/${observationId}/image`);
  }
}

export const apiClient = new ApiClient();
