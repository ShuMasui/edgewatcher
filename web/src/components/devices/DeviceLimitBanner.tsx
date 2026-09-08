import React from 'react';

interface DeviceLimitBannerProps {
  limit: number;
}

export const DeviceLimitBanner: React.FC<DeviceLimitBannerProps> = ({ limit }) => {
  return (
    <div className="banner" role="alert" data-testid="device-limit-banner">
      <span className="label">上限</span>
      <div>
        <strong>登録できる端末は {limit} 台までです。</strong>
        <br />
        追加するには、先に既存の端末を削除してください。ペアリング待ちの端末も1台として数えます。
      </div>
    </div>
  );
};
