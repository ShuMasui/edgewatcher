import React from 'react';
import { Button } from '../common/Button';

interface PairingDoneStepProps {
  deviceName: string;
  onClose: () => void;
}

export const PairingDoneStep: React.FC<PairingDoneStepProps> = ({
  deviceName,
  onClose,
}) => {
  return (
    <div data-testid="pairing-done-step">
      <div className="success-mark" data-testid="pairing-success-check">
        ✓
      </div>
      <h4>「{deviceName}」が接続しました</h4>
      <p>
        まもなく最初の画像が届きます。
        <br />
        送信間隔は端末一覧から変更できます。
      </p>
      <div className="actions" style={{ justifyContent: 'center' }}>
        <Button type="button" variant="primary" onClick={onClose} data-testid="pairing-done-close-btn">
          閉じる
        </Button>
      </div>
    </div>
  );
};
