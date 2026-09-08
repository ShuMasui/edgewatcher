import React from 'react';
import { Button } from '../common/Button';

interface PairingExpiredStepProps {
  onCancel: () => void;
  onReissue: () => Promise<void>;
  isLoading?: boolean;
}

export const PairingExpiredStep: React.FC<PairingExpiredStepProps> = ({
  onCancel,
  onReissue,
  isLoading = false,
}) => {
  return (
    <div data-testid="pairing-expired-step">
      <h4>有効期限が切れました</h4>
      <p>
        QR コードは5分で無効になります。
        <br />
        もう一度発行してください。
      </p>

      <div className="qr-wrapper">
        <div className="qr qr-void" data-testid="expired-qr-placeholder">
          期限切れ
        </div>
      </div>

      <div className="actions" style={{ justifyContent: 'center' }}>
        <Button type="button" onClick={onCancel} disabled={isLoading}>
          キャンセル
        </Button>
        <Button
          type="button"
          variant="primary"
          onClick={onReissue}
          disabled={isLoading}
          data-testid="reissue-qr-btn"
        >
          {isLoading ? '再発行中...' : 'QR を再発行'}
        </Button>
      </div>
    </div>
  );
};
