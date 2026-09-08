import React from 'react';
import { Device } from '../../types/domain';
import { DeviceCard } from './DeviceCard';

interface DeviceGridProps {
  devices: Device[];
}

export const DeviceGrid: React.FC<DeviceGridProps> = ({ devices }) => {
  return (
    <div className="grid" data-testid="device-grid">
      {devices.map((device) => (
        <DeviceCard key={device.deviceId} device={device} />
      ))}
    </div>
  );
};
