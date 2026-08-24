import { AppConfig, Device, Observation, PairingSession } from '../types/domain';
import { generateMockImageSvg } from './image-generator';
import { formatDateParam, isSameDay } from '../utils/date';

const STORAGE_KEY = 'edgewatcher_mock_devices';

const DEFAULT_CONFIG: AppConfig = {
  retentionDays: 7,
  deviceLimit: 10,
  intervalOptions: [5, 10, 15],
};

function getInitialDevices(): Device[] {
  const now = new Date();
  const twoMinsAgo = new Date(now.getTime() - 2 * 60 * 1000).toISOString();
  const fourMinsAgo = new Date(now.getTime() - 4 * 60 * 1000).toISOString();
  const threeHoursAgo = new Date(now.getTime() - 3 * 60 * 60 * 1000).toISOString();

  return [
    {
      deviceId: '01JDEV001GENKAN0000000001',
      ownerId: 'mock-user-owner-001',
      name: '玄関',
      status: 'PAIRED',
      interval: 5,
      lastReceivedAt: twoMinsAgo,
      latestCapturedAt: twoMinsAgo,
      latestThumbnailUrl: generateMockImageSvg(3, 14),
      createdAt: new Date(now.getTime() - 10 * 24 * 60 * 60 * 1000).toISOString(),
    },
    {
      deviceId: '01JDEV002URANIWA000000002',
      ownerId: 'mock-user-owner-001',
      name: '裏庭',
      status: 'PAIRED',
      interval: 5,
      lastReceivedAt: fourMinsAgo,
      latestCapturedAt: fourMinsAgo,
      latestThumbnailUrl: generateMockImageSvg(8, 14),
      createdAt: new Date(now.getTime() - 5 * 24 * 60 * 60 * 1000).toISOString(),
    },
    {
      deviceId: '01JDEV003CHUSHA0000000003',
      ownerId: 'mock-user-owner-001',
      name: '駐車場',
      status: 'PAIRED',
      interval: 10,
      lastReceivedAt: threeHoursAgo,
      latestCapturedAt: threeHoursAgo,
      latestThumbnailUrl: generateMockImageSvg(1, 21),
      createdAt: new Date(now.getTime() - 2 * 24 * 60 * 60 * 1000).toISOString(),
    },
  ];
}

class MockServer {
  private devices: Device[];
  private config: AppConfig;

  constructor() {
    this.config = { ...DEFAULT_CONFIG };
    const saved = localStorage.getItem(STORAGE_KEY);
    if (saved) {
      try {
        this.devices = JSON.parse(saved);
      } catch {
        this.devices = getInitialDevices();
      }
    } else {
      this.devices = getInitialDevices();
    }
  }

  private save() {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(this.devices));
  }

  public resetToDefault() {
    this.devices = getInitialDevices();
    this.save();
  }

  public setFullDevices() {
    const base = getInitialDevices();
    const now = new Date();
    this.devices = [
      ...base,
      {
        deviceId: 'd4',
        ownerId: 'mock-user-owner-001',
        name: '物置',
        status: 'PAIRED',
        interval: 5,
        lastReceivedAt: new Date(now.getTime() - 1 * 60 * 1000).toISOString(),
        latestCapturedAt: new Date(now.getTime() - 1 * 60 * 1000).toISOString(),
        latestThumbnailUrl: generateMockImageSvg(2, 14),
      },
      {
        deviceId: 'd5',
        ownerId: 'mock-user-owner-001',
        name: '倉庫',
        status: 'PAIRED',
        interval: 10,
        lastReceivedAt: new Date(now.getTime() - 6 * 60 * 1000).toISOString(),
        latestCapturedAt: new Date(now.getTime() - 6 * 60 * 1000).toISOString(),
        latestThumbnailUrl: generateMockImageSvg(5, 14),
      },
      {
        deviceId: 'd6',
        ownerId: 'mock-user-owner-001',
        name: '東側通路',
        status: 'PAIRED',
        interval: 15,
        lastReceivedAt: new Date(now.getTime() - 2 * 24 * 60 * 60 * 1000).toISOString(),
        latestCapturedAt: new Date(now.getTime() - 2 * 24 * 60 * 60 * 1000).toISOString(),
        latestThumbnailUrl: generateMockImageSvg(9, 20),
      },
      {
        deviceId: 'd7',
        ownerId: 'mock-user-owner-001',
        name: 'temp-01',
        status: 'PENDING',
        interval: 5,
        activePairingSession: {
          pairingCode: 'PAIRINGCODE007',
          expiresAt: Math.floor(Date.now() / 1000) + 270,
          status: 'PENDING',
        },
      },
      {
        deviceId: 'd8',
        ownerId: 'mock-user-owner-001',
        name: 'temp-02',
        status: 'PENDING',
        interval: 5,
        activePairingSession: {
          pairingCode: 'PAIRINGCODE008',
          expiresAt: Math.floor(Date.now() / 1000) + 200,
          status: 'PENDING',
        },
      },
      {
        deviceId: 'd9',
        ownerId: 'mock-user-owner-001',
        name: '旧・玄関',
        status: 'DISCONNECTED',
        interval: 5,
        lastReceivedAt: new Date(now.getTime() - 12 * 24 * 60 * 60 * 1000).toISOString(),
      },
      {
        deviceId: 'd10',
        ownerId: 'mock-user-owner-001',
        name: 'ベランダ',
        status: 'PAIRED',
        interval: 5,
        lastReceivedAt: new Date(now.getTime() - 3 * 60 * 1000).toISOString(),
        latestCapturedAt: new Date(now.getTime() - 3 * 60 * 1000).toISOString(),
        latestThumbnailUrl: generateMockImageSvg(6, 14),
      },
    ];
    this.save();
  }

  async getAppConfig(): Promise<AppConfig> {
    return { ...this.config };
  }

  async getDevices(): Promise<Device[]> {
    // Only return non-archived devices (03-web.md §1.3)
    return this.devices.filter((d) => d.status !== 'ARCHIVED');
  }

  async getDeviceById(deviceId: string): Promise<Device | null> {
    const d = this.devices.find((item) => item.deviceId === deviceId);
    if (!d || d.status === 'ARCHIVED') return null;
    return d;
  }

  async createDevice(name: string): Promise<{ device: Device; pairingSession: PairingSession }> {
    const activeCount = this.devices.filter((d) => d.status !== 'ARCHIVED').length;
    if (activeCount >= this.config.deviceLimit) {
      const err: any = new Error('Device limit exceeded');
      err.statusCode = 429;
      err.code = 'DEVICE_LIMIT_EXCEEDED';
      throw err;
    }

    const deviceId = '01JDEV' + Math.random().toString(36).substring(2, 10).toUpperCase();
    const pairingCode = 'PAIR_' + Math.random().toString(36).substring(2, 12).toUpperCase();
    const expiresAt = Math.floor(Date.now() / 1000) + 300; // 5 minutes

    const session: PairingSession = {
      pairingCode,
      expiresAt,
      status: 'PENDING',
      createdAt: new Date().toISOString(),
    };

    const device: Device = {
      deviceId,
      ownerId: 'mock-user-owner-001',
      name: name.trim() || '名称未設定',
      status: 'PENDING',
      interval: 5,
      createdAt: new Date().toISOString(),
      activePairingSession: session,
    };

    this.devices.push(device);
    this.save();

    // Auto-simulate device scanning QR after 3.5 seconds in mock environment
    setTimeout(() => {
      this.simulatePairingSuccess(deviceId);
    }, 3500);

    return { device, pairingSession: session };
  }

  async createPairingSession(deviceId: string): Promise<PairingSession> {
    const device = this.devices.find((d) => d.deviceId === deviceId);
    if (!device) {
      const err: any = new Error('Device not found');
      err.statusCode = 404;
      throw err;
    }

    const pairingCode = 'REPAIR_' + Math.random().toString(36).substring(2, 12).toUpperCase();
    const expiresAt = Math.floor(Date.now() / 1000) + 300;

    const session: PairingSession = {
      pairingCode,
      expiresAt,
      status: 'PENDING',
      createdAt: new Date().toISOString(),
    };

    device.activePairingSession = session;
    this.save();

    setTimeout(() => {
      this.simulatePairingSuccess(deviceId);
    }, 3500);

    return session;
  }

  async getLatestPairingSession(deviceId: string): Promise<PairingSession | null> {
    const device = this.devices.find((d) => d.deviceId === deviceId);
    if (!device || !device.activePairingSession) {
      return null;
    }
    return device.activePairingSession;
  }

  simulatePairingSuccess(deviceId: string) {
    const device = this.devices.find((d) => d.deviceId === deviceId);
    if (device && device.activePairingSession) {
      device.status = 'PAIRED';
      device.activePairingSession.status = 'CONSUMED';
      const now = new Date().toISOString();
      device.lastReceivedAt = now;
      device.latestCapturedAt = now;
      device.latestThumbnailUrl = generateMockImageSvg(Math.floor(Math.random() * 10), new Date().getHours());
      this.save();
    }
  }

  async disconnectDevice(deviceId: string): Promise<Device> {
    const device = this.devices.find((d) => d.deviceId === deviceId);
    if (!device) {
      const err: any = new Error('Device not found');
      err.statusCode = 404;
      throw err;
    }
    device.status = 'DISCONNECTED';
    device.activePairingSession = null;
    this.save();
    return device;
  }

  async updateDevice(deviceId: string, updates: { name?: string; interval?: number }): Promise<Device> {
    const device = this.devices.find((d) => d.deviceId === deviceId);
    if (!device) {
      const err: any = new Error('Device not found');
      err.statusCode = 404;
      throw err;
    }
    if (updates.name !== undefined) {
      device.name = updates.name.trim();
    }
    if (updates.interval !== undefined) {
      device.interval = updates.interval;
    }
    this.save();
    return device;
  }

  async deleteDevice(deviceId: string): Promise<void> {
    const device = this.devices.find((d) => d.deviceId === deviceId);
    if (!device) {
      const err: any = new Error('Device not found');
      err.statusCode = 404;
      throw err;
    }
    // Logical deletion: status = ARCHIVED (03-web.md §1.3, 05-backend.md §1.2)
    device.status = 'ARCHIVED';
    device.archivedAt = new Date().toISOString();
    device.activePairingSession = null;
    this.save();
  }

  async getObservations(deviceId: string, dateStr: string): Promise<Observation[]> {
    const device = await this.getDeviceById(deviceId);
    if (!device) {
      const err: any = new Error('Device not found');
      err.statusCode = 404;
      err.code = 'DEVICE_NOT_FOUND';
      throw err;
    }

    const today = new Date();
    const queryDate = new Date(dateStr);

    // If querying an empty day (e.g. 3 days ago or specific seed), simulate empty
    const seed = device.name.charCodeAt(0) || 1;

    // Generate observations for this day
    const isToday = isSameDay(queryDate, today);
    const endHour = isToday ? today.getHours() : 23;
    const endMinute = isToday ? today.getMinutes() : 55;

    const intervalMin = device.interval || 5;
    const observations: Observation[] = [];

    const baseYear = queryDate.getFullYear();
    const baseMonth = queryDate.getMonth();
    const baseDay = queryDate.getDate();

    let obsIndex = 0;
    for (let h = 0; h <= endHour; h++) {
      const maxMin = h === endHour ? endMinute : 59;
      for (let m = 0; m <= maxMin; m += intervalMin) {
        obsIndex++;
        const obsDate = new Date(baseYear, baseMonth, baseDay, h, m, 0);
        const iso = obsDate.toISOString();
        const obsId = `01JOBS${formatDateParam(queryDate).replace(/-/g, '')}${String(obsIndex).padStart(4, '0')}`;
        const svg = generateMockImageSvg(seed + obsIndex, h);

        observations.push({
          observationId: obsId,
          deviceId: device.deviceId,
          capturedAt: iso,
          thumbnailUrl: svg,
          imageUrl: svg,
          lat: 35.6812,
          lng: 139.7671,
        });
      }
    }

    return observations;
  }

  async getObservationImage(observationId: string): Promise<{ observationId: string; imageUrl: string; expiresAt: number }> {
    const hour = parseInt(observationId.slice(-2), 10) % 24 || 14;
    const svg = generateMockImageSvg(observationId.charCodeAt(observationId.length - 1), hour);
    return {
      observationId,
      imageUrl: svg,
      expiresAt: Math.floor(Date.now() / 1000) + 900, // 15 minutes TTL
    };
  }
}

export const mockServer = new MockServer();
