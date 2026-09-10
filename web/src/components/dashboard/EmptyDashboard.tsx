import React from 'react';
import { Button } from '../common/Button';

interface EmptyDashboardProps {
  onAddDevice: () => void;
}

export const EmptyDashboard: React.FC<EmptyDashboardProps> = ({ onAddDevice }) => {
  return (
    <div className="empty" data-testid="empty-dashboard">
      <h4>まだ端末がありません</h4>
      <p>
        Android 端末に EdgeWatcher をインストールし、
        <br />
        QR コードを読み取らせるとここに画像が並びます。
      </p>
      <Button variant="primary" onClick={onAddDevice} data-testid="empty-add-device-btn">
        端末を追加
      </Button>
    </div>
  );
};
