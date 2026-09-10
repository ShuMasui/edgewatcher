import { describe, it, expect } from 'vitest';
import { getDerivedDeviceStatus, getStatusBadgeInfo } from '../status';
import { Device } from '../../types/domain';

describe('status utilities', () => {
  const baseDevice: Device = {
    deviceId: 'dev-1',
    ownerId: 'owner-1',
    name: '玄関',
    status: 'PAIRED',
    interval: 5,
    lastReceivedAt: new Date().toISOString(),
  };

  it('should return pending when raw status is PENDING', () => {
    const dev: Device = { ...baseDevice, status: 'PENDING' };
    expect(getDerivedDeviceStatus(dev)).toBe('pending');
  });

  it('should return pending when activePairingSession is PENDING regardless of status', () => {
    const dev: Device = {
      ...baseDevice,
      status: 'DISCONNECTED',
      activePairingSession: {
        pairingCode: 'CODE123',
        expiresAt: Date.now() / 1000 + 300,
        status: 'PENDING',
      },
    };
    expect(getDerivedDeviceStatus(dev)).toBe('pending');
  });

  it('should return disconnected when raw status is DISCONNECTED without active session', () => {
    const dev: Device = { ...baseDevice, status: 'DISCONNECTED', activePairingSession: null };
    expect(getDerivedDeviceStatus(dev)).toBe('disconnected');
  });

  it('should return paired when PAIRED and lastReceivedAt is within 3x interval (15 mins for 5 min interval)', () => {
    const now = new Date('2026-08-24T12:00:00Z');
    const dev: Device = {
      ...baseDevice,
      status: 'PAIRED',
      interval: 5,
      lastReceivedAt: new Date('2026-08-24T11:50:00Z').toISOString(), // 10 mins ago <= 15 mins
    };
    expect(getDerivedDeviceStatus(dev, now)).toBe('paired');
  });

  it('should return stale when PAIRED and lastReceivedAt is beyond 3x interval (e.g. 20 mins for 5 min interval)', () => {
    const now = new Date('2026-08-24T12:00:00Z');
    const dev: Device = {
      ...baseDevice,
      status: 'PAIRED',
      interval: 5,
      lastReceivedAt: new Date('2026-08-24T11:35:00Z').toISOString(), // 25 mins ago > 15 mins
    };
    expect(getDerivedDeviceStatus(dev, now)).toBe('stale');
  });

  it('should return correct badge labels and dot classes', () => {
    expect(getStatusBadgeInfo('paired')).toEqual({ label: '接続中', dotClass: 'ok' });
    expect(getStatusBadgeInfo('stale')).toEqual({ label: '応答なし', dotClass: 'warn' });
    expect(getStatusBadgeInfo('pending')).toEqual({ label: 'ペアリング待ち', dotClass: 'pending' });
    expect(getStatusBadgeInfo('disconnected')).toEqual({ label: '切断済み', dotClass: 'disconnected' });
  });
});
