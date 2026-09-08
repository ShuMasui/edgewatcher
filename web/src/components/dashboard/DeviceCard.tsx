import React from 'react';
import { useNavigate } from 'react-router-dom';
import { Device } from '../../types/domain';
import { getDerivedDeviceStatus } from '../../utils/status';
import { formatRelativeTime } from '../../utils/date';
import { Badge } from '../common/Badge';

interface DeviceCardProps {
  device: Device;
}

export const DeviceCard: React.FC<DeviceCardProps> = ({ device }) => {
  const navigate = useNavigate();
  const status = getDerivedDeviceStatus(device);

  const handleClick = () => {
    navigate(`/devices/${device.deviceId}/history`);
  };

  const renderVisual = () => {
    if (status === 'pending') {
      return (
        <div className="placeholder">
          <span>QR コードの読み取りを待っています</span>
        </div>
      );
    }

    if (status === 'disconnected') {
      return (
        <div className="placeholder">
          <span>切断済み。再ペアリングすると再開します</span>
        </div>
      );
    }

    if (device.latestThumbnailUrl) {
      return (
        <div className="shot">
          <img
            src={device.latestThumbnailUrl}
            alt={`${device.name}の最新画像`}
            loading="lazy"
          />
          <span className="stamp">{formatRelativeTime(device.latestCapturedAt || device.lastReceivedAt)}</span>
        </div>
      );
    }

    return (
      <div className="placeholder">
        <span>まだ画像が届いていません</span>
      </div>
    );
  };

  return (
    <div
      className={`card ${status === 'stale' ? 'is-down' : ''}`}
      onClick={handleClick}
      role="button"
      tabIndex={0}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          handleClick();
        }
      }}
      data-testid={`device-card-${device.deviceId}`}
    >
      {renderVisual()}
      <div className="card-body">
        <span className="name" title={device.name}>
          {device.name}
        </span>
        <span className="push">
          <Badge status={status} />
        </span>
      </div>
    </div>
  );
};
