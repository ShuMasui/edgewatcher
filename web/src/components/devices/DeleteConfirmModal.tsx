import React, { useState } from 'react';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { FormError } from '../common/FormError';

interface DeleteConfirmModalProps {
  isOpen: boolean;
  deviceName: string;
  onClose: () => void;
  onConfirm: () => Promise<void>;
}

export const DeleteConfirmModal: React.FC<DeleteConfirmModalProps> = ({
  isOpen,
  deviceName,
  onClose,
  onConfirm,
}) => {
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleDelete = async () => {
    setIsLoading(true);
    setError(null);
    try {
      await onConfirm();
      onClose();
    } catch (err: any) {
      // 03-web.md §1.10.4: Error shown in dialog, dialog stays open
      setError('削除できませんでした。時間をおいてもう一度お試しください。');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} align="left">
      <h4>「{deviceName}」を削除しますか?</h4>
      {error && <FormError>{error}</FormError>}
      <p>
        この端末の観測画像もすべて削除されます。取り消せません。
        <br />
        一時的に止めたいだけなら「セッション切断」を使ってください。切断なら履歴は残り、再ペアリングで復帰できます。
      </p>
      <div className="actions">
        <Button type="button" onClick={onClose} disabled={isLoading}>
          キャンセル
        </Button>
        <Button
          type="button"
          variant="danger"
          onClick={handleDelete}
          disabled={isLoading}
          data-testid="confirm-delete-btn"
        >
          {isLoading ? '削除中...' : error ? '再試行' : '削除する'}
        </Button>
      </div>
    </Modal>
  );
};
