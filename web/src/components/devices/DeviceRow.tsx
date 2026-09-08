import React, { useState } from 'react';
import { Device } from '../../types/domain';
import { getDerivedDeviceStatus } from '../../utils/status';
import { formatRelativeTime } from '../../utils/date';
import { Badge } from '../common/Badge';
import { Button } from '../common/Button';
import { KebabMenu } from './KebabMenu';

interface DeviceRowProps {
  device: Device;
  intervalOptions: number[];
  onShowQr: (device: Device) => void;
  onRepair: (device: Device) => void;
  onDisconnect: (device: Device) => Promise<void>;
  onRename: (device: Device) => void;
  onDelete: (device: Device) => void;
  onUpdateInterval: (deviceId: string, interval: number) => Promise<void>;
}

export const DeviceRow: React.FC<DeviceRowProps> = ({
  device,
  intervalOptions,
  onShowQr,
  onRepair,
  onDisconnect,
  onRename,
  onDelete,
  onUpdateInterval,
}) => {
  const status = getDerivedDeviceStatus(device);
  const [selectedInterval, setSelectedInterval] = useState<number>(device.interval || 5);
  const [intervalError, setIntervalError] = useState<string | null>(null);
  const [isUpdatingInterval, setIsUpdatingInterval] = useState(false);
  const [isDisconnecting, setIsDisconnecting] = useState(false);

  const handleIntervalChange = async (e: React.ChangeEvent<HTMLSelectElement>) => {
    const newInterval = Number(e.target.value);
    const oldInterval = selectedInterval;
    setSelectedInterval(newInterval);
    setIntervalError(null);
    setIsUpdatingInterval(true);

    try {
      await onUpdateInterval(device.deviceId, newInterval);
    } catch (err: any) {
      // 03-web.md §1.10.4: Show inline error, revert select to original value
      setSelectedInterval(oldInterval);
      setIntervalError('変更できませんでした。再試行してください。');
    } finally {
      setIsUpdatingInterval(false);
    }
  };

  const handleDisconnectClick = async () => {
    setIsDisconnecting(true);
    try {
      await onDisconnect(device);
    } finally {
      setIsDisconnecting(false);
    }
  };

  const renderActionButton = () => {
    const hasActivePairing = device.activePairingSession?.status === 'PENDING';

    if (hasActivePairing || status === 'pending') {
      return (
        <Button
          size="sm"
          variant="primary"
          onClick={() => onShowQr(device)}
          data-testid={`action-qr-${device.deviceId}`}
        >
          QR を表示
        </Button>
      );
    }

    if (status === 'disconnected') {
      return (
        <Button
          size="sm"
          onClick={() => onRepair(device)}
          data-testid={`action-repair-${device.deviceId}`}
        >
          再ペアリング
        </Button>
      );
    }

    return (
      <Button
        size="sm"
        onClick={handleDisconnectClick}
        disabled={isDisconnecting}
        data-testid={`action-disconnect-${device.deviceId}`}
      >
        {isDisconnecting ? '切断中...' : 'セッション切断'}
      </Button>
    );
  };

  return (
    <tr data-testid={`device-row-${device.deviceId}`}>
      <td>
        <strong>{device.name}</strong>
      </td>
      <td>
        <Badge status={status} />
      </td>
      <td className="num">
        {formatRelativeTime(device.lastReceivedAt)}
      </td>
      <td>
        {status === 'pending' ? (
          <span className="num">—</span>
        ) : (
          <div>
            <select
              value={selectedInterval}
              onChange={handleIntervalChange}
              disabled={isUpdatingInterval}
              data-testid={`interval-select-${device.deviceId}`}
              aria-label={`${device.name}の送信間隔`}
            >
              {intervalOptions.map((opt) => (
                <option key={opt} value={opt}>
                  {opt}分
                </option>
              ))}
            </select>
            {intervalError && (
              <div className="row-error" data-testid={`interval-error-${device.deviceId}`}>
                {intervalError}
              </div>
            )}
          </div>
        )}
      </td>
      <td>
        <div className="row-actions">
          {renderActionButton()}
          <KebabMenu
            onRename={() => onRename(device)}
            onDelete={() => onDelete(device)}
          />
        </div>
      </td>
    </tr>
  );
};
