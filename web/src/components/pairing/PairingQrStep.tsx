import React, { useState, useEffect } from 'react';
import { generateQrDataUrl } from '../../utils/qr';
import { formatCountdown } from '../../utils/date';
import { usePairingSessionPolling } from '../../hooks/use-pairing';
import { Button } from '../common/Button';

interface PairingQrStepProps {
  deviceId: string;
  deviceName: string;
  pairingCode: string;
  expiresAt: number; // epoch seconds
  isRepairing?: boolean;
  onCancel: () => void;
  onExpired: () => void;
  onSuccess: () => void;
}

export const PairingQrStep: React.FC<PairingQrStepProps> = ({
  deviceId,
  deviceName,
  pairingCode,
  expiresAt,
  isRepairing = false,
  onCancel,
  onExpired,
  onSuccess,
}) => {
  const [qrDataUrl, setQrDataUrl] = useState<string>('');
  const [secondsRemaining, setSecondsRemaining] = useState<number>(() => {
    const diff = expiresAt - Math.floor(Date.now() / 1000);
    return Math.max(0, diff);
  });

  // Generate QR code for payload "ew1:<pairingCode>"
  useEffect(() => {
    generateQrDataUrl(`ew1:${pairingCode}`)
      .then((url) => setQrDataUrl(url))
      .catch((err) => console.error('Failed to generate QR:', err));
  }, [pairingCode]);

  // 1-second interval countdown timer
  useEffect(() => {
    const interval = setInterval(() => {
      const remaining = expiresAt - Math.floor(Date.now() / 1000);
      if (remaining <= 0) {
        setSecondsRemaining(0);
        clearInterval(interval);
        onExpired();
      } else {
        setSecondsRemaining(remaining);
      }
    }, 1000);

    return () => clearInterval(interval);
  }, [expiresAt, onExpired]);

  // 2-second polling for pairing session status
  const { data: session } = usePairingSessionPolling(deviceId, true);

  useEffect(() => {
    if (session?.status === 'CONSUMED') {
      onSuccess();
    }
  }, [session, onSuccess]);

  return (
    <div data-testid="pairing-qr-step">
      <h4>{isRepairing ? `「${deviceName}」を再ペアリング` : `「${deviceName}」をペアリング`}</h4>
      <p>
        {isRepairing ? (
          <>
            新しい QR コードを発行しました。
            <br />
            過去の画像履歴はそのまま引き継がれます。
          </>
        ) : (
          <>
            Android 端末の EdgeWatcher アプリで、
            <br />
            この QR コードを読み取ってください。
          </>
        )}
      </p>

      <div className="qr-wrapper" data-testid="qr-container">
        {qrDataUrl ? (
          <img src={qrDataUrl} alt="Pairing QR Code" className="qr" data-testid="pairing-qr-image" />
        ) : (
          <div style={{ width: 170, height: 170, display: 'grid', placeItems: 'center', color: '#888' }}>
            QR 生成中...
          </div>
        )}
      </div>

      <div className="countdown" data-testid="pairing-countdown">
        残り {formatCountdown(secondsRemaining)}
      </div>

      <div className="actions" style={{ justifyContent: 'center' }}>
        <Button type="button" onClick={onCancel} data-testid="pairing-cancel-btn">
          キャンセル
        </Button>
      </div>
    </div>
  );
};
