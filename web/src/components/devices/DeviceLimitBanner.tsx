import React from 'react';

interface DeviceLimitBannerProps {
  limit: number;
}

export const DeviceLimitBanner: React.FC<DeviceLimitBannerProps> = ({ limit }) => {
  return (
    <div className="banner" role="alert" data-testid="device-limit-banner">
      <span>⚠</span>
      <div>
        <strong>端末の上限({limit}台)に達しています。</strong>
        <br />
        新しい端末を追加するには、既存の端末を削除してください。
        ペアリング待ちの端末も1台として数えます。
      </div>
    </div>
  );
};
