import React from 'react';
import { DeviceDerivedStatus } from '../../types/domain';
import { getStatusBadgeInfo } from '../../utils/status';

interface BadgeProps {
  status: DeviceDerivedStatus;
}

export const Badge: React.FC<BadgeProps> = ({ status }) => {
  const { label, dotClass } = getStatusBadgeInfo(status);

  return (
    <span className="badge" data-testid={`badge-${status}`}>
      <span className={`dot ${dotClass}`} />
      {label}
    </span>
  );
};
