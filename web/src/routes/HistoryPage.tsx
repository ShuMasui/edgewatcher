import React, { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Shell } from '../components/layout/Shell';
import { useDevice } from '../hooks/use-devices';
import { useAppConfig } from '../hooks/use-app-config';
import { useObservations } from '../hooks/use-observations';
import { DateNavigator } from '../components/history/DateNavigator';
import { HistoryViewer } from '../components/history/HistoryViewer';
import { InlineError } from '../components/common/InlineError';
import { Button } from '../components/common/Button';
import { addDays, formatDateParam, isSameDay } from '../utils/date';

export const HistoryPage: React.FC = () => {
  const { deviceId = '' } = useParams<{ deviceId: string }>();
  const navigate = useNavigate();

  const { config } = useAppConfig();
  const { device, isLoading: isDeviceLoading, isNotFound } = useDevice(deviceId);

  const [currentDate, setCurrentDate] = useState<Date>(new Date());
  const dateStr = formatDateParam(currentDate);

  const {
    observations,
    isLoading: isObsLoading,
    isError,
    refetch,
    handleImageError,
  } = useObservations(deviceId, dateStr);

  const today = new Date();
  const retentionDays = config.retentionDays || 7;
  // Bounded oldest date: retentionDays days ago (e.g. 7 days ago, or today for retentionDays=1)
  const oldestDate = addDays(today, -(retentionDays - 1));

  const atOldest = isSameDay(currentDate, oldestDate) || currentDate < oldestDate;
  const atNewest = isSameDay(currentDate, today) || currentDate > today;

  const handlePrevDate = () => {
    if (!atOldest) {
      setCurrentDate((prev) => addDays(prev, -1));
    }
  };

  const handleNextDate = () => {
    if (!atNewest) {
      setCurrentDate((prev) => addDays(prev, 1));
    }
  };

  if (!isDeviceLoading && isNotFound) {
    return (
      <Shell>
        <div className="topbar">
          <Button size="sm" onClick={() => navigate('/devices')}>
            ← 戻る
          </Button>
          <h3>端末</h3>
        </div>
        <div className="content">
          <InlineError
            mark="?"
            title="この端末は見つかりません"
            message="削除されたか、アクセスする権限がありません。"
            actionButton={
              <Button variant="primary" onClick={() => navigate('/devices')} data-testid="back-to-devices-btn">
                端末一覧へ戻る
              </Button>
            }
          />
        </div>
      </Shell>
    );
  }

  return (
    <Shell>
      <div className="topbar" data-testid="history-topbar">
        <Button size="sm" onClick={() => navigate(-1)} data-testid="back-button">
          ← 戻る
        </Button>
        <h3>{device?.name || '端末'}</h3>
        <DateNavigator
          currentDate={currentDate}
          atOldest={atOldest}
          atNewest={atNewest}
          onPrevDate={handlePrevDate}
          onNextDate={handleNextDate}
        />
      </div>

      <div className="content" data-testid="history-content">
        {isDeviceLoading || isObsLoading ? (
          <div style={{ padding: 40, textAlign: 'center', color: 'var(--muted)' }}>
            読み込み中...
          </div>
        ) : isError ? (
          <InlineError
            title="履歴を読み込めませんでした"
            message="通信に失敗しました。しばらく待ってからもう一度お試しください。"
            onRetry={() => refetch()}
          />
        ) : (
          <HistoryViewer
            observations={observations}
            retentionDays={retentionDays}
            atOldest={atOldest}
            onImageError={handleImageError}
          />
        )}
      </div>
    </Shell>
  );
};
