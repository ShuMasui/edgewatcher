import React, { useState } from 'react';
import { Shell } from '../components/layout/Shell';
import { useDevices } from '../hooks/use-devices';
import { DeviceGrid } from '../components/dashboard/DeviceGrid';
import { EmptyDashboard } from '../components/dashboard/EmptyDashboard';
import { InlineError } from '../components/common/InlineError';
import { PairingModal } from '../components/pairing/PairingModal';
import { formatTime } from '../utils/date';

export const DashboardPage: React.FC = () => {
  const {
    devices,
    isLoading,
    isInitialError,
    isPollingError,
    lastSuccessTime,
    refetch,
  } = useDevices();

  const [isPairingModalOpen, setIsPairingModalOpen] = useState(false);

  return (
    <Shell>
      <div className="topbar" data-testid="dashboard-topbar">
        <h3>最新画像</h3>
        {!isLoading && !isInitialError && (
          <span className="count" data-testid="device-count">
            {devices.length} 台
          </span>
        )}
        {isPollingError && (
          <span className="stale" data-testid="stale-indicator">
            <span className="dot" />
            更新できていません(最終更新 {lastSuccessTime ? formatTime(lastSuccessTime) : ''})
          </span>
        )}
      </div>

      <div className="content" data-testid="dashboard-content">
        {isLoading && devices.length === 0 ? (
          <div style={{ padding: 40, textAlign: 'center', color: 'var(--muted)' }}>
            読み込み中...
          </div>
        ) : isInitialError ? (
          <InlineError
            title="読み込めませんでした"
            message="通信に失敗しました。しばらく待ってからもう一度お試しください。"
            onRetry={() => refetch()}
          />
        ) : devices.length === 0 ? (
          <EmptyDashboard onAddDevice={() => setIsPairingModalOpen(true)} />
        ) : (
          <DeviceGrid devices={devices} />
        )}
      </div>

      <PairingModal
        isOpen={isPairingModalOpen}
        onClose={() => setIsPairingModalOpen(false)}
      />
    </Shell>
  );
};
