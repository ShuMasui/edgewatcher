import React, { useState } from 'react';
import { Button } from '../common/Button';
import { FormError } from '../common/FormError';

interface PairingNameStepProps {
  onCancel: () => void;
  onSubmitName: (name: string) => Promise<void>;
}

export const PairingNameStep: React.FC<PairingNameStepProps> = ({
  onCancel,
  onSubmitName,
}) => {
  const [name, setName] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [is429, setIs429] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;

    setIsLoading(true);
    setError(null);
    setIs429(false);

    try {
      await onSubmitName(name.trim());
    } catch (err: any) {
      if (err?.statusCode === 429 || err?.code === 'DEVICE_LIMIT_EXCEEDED') {
        setIs429(true);
        setError('端末の上限に達しています。別のタブで端末が追加された可能性があります。');
      } else {
        setError('端末の作成に失敗しました。時間をおいて再試行してください。');
      }
    } finally {
      setIsLoading(false);
    }
  };

  if (is429) {
    return (
      <div data-testid="pairing-429-view">
        <h4>端末を追加できませんでした</h4>
        <FormError>
          端末の上限に達しています。
          <br />
          別のタブで端末が追加された可能性があります。
        </FormError>
        <p>追加するには、既存の端末を削除してください。</p>
        <div className="actions">
          <Button type="button" onClick={onCancel}>
            閉じる
          </Button>
          <Button type="button" variant="primary" onClick={onCancel}>
            端末一覧へ
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div data-testid="pairing-name-step">
      <h4>端末を追加</h4>
      <p>設置場所が分かる名前を付けてください。あとから変更できます。</p>
      {error && <FormError>{error}</FormError>}
      <form onSubmit={handleSubmit} data-testid="pairing-name-form">
        <div className="field">
          <label htmlFor="pairing-device-name-input">端末名</label>
          <input
            id="pairing-device-name-input"
            type="text"
            placeholder="例: 玄関、物置"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            disabled={isLoading}
            data-testid="pairing-name-input"
          />
        </div>
        <div className="actions">
          <Button type="button" onClick={onCancel} disabled={isLoading}>
            キャンセル
          </Button>
          <Button
            type="submit"
            variant="primary"
            disabled={isLoading || !name.trim()}
            data-testid="pairing-name-submit-btn"
          >
            {isLoading ? '発行中...' : 'QR コードを表示'}
          </Button>
        </div>
      </form>
    </div>
  );
};
