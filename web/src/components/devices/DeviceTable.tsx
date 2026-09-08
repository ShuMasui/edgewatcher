import React from 'react';
import { Device } from '../../types/domain';
import { DeviceRow } from './DeviceRow';

interface DeviceTableProps {
  devices: Device[];
  intervalOptions: number[];
  onShowQr: (device: Device) => void;
  onRepair: (device: Device) => void;
  onDisconnect: (device: Device) => Promise<void>;
  onRename: (device: Device) => void;
  onDelete: (device: Device) => void;
  onUpdateInterval: (deviceId: string, interval: number) => Promise<void>;
}

export const DeviceTable: React.FC<DeviceTableProps> = ({
  devices,
  intervalOptions,
  onShowQr,
  onRepair,
  onDisconnect,
  onRename,
  onDelete,
  onUpdateInterval,
}) => {
  return (
    <table data-testid="device-table">
      <thead>
        <tr>
          <th>端末名</th>
          <th>状態</th>
          <th>最終受信</th>
          <th>送信間隔</th>
          <th />
        </tr>
      </thead>
      <tbody>
        {devices.map((device) => (
          <DeviceRow
            key={device.deviceId}
            device={device}
            intervalOptions={intervalOptions}
            onShowQr={onShowQr}
            onRepair={onRepair}
            onDisconnect={onDisconnect}
            onRename={onRename}
            onDelete={onDelete}
            onUpdateInterval={onUpdateInterval}
          />
        ))}
      </tbody>
    </table>
  );
};
