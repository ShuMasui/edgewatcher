import React, { useState, useEffect } from 'react';
import { Modal } from '../common/Modal';
import { Button } from '../common/Button';
import { FormError } from '../common/FormError';

interface RenameModalProps {
  isOpen: boolean;
  initialName: string;
  onClose: () => void;
  onSave: (newName: string) => Promise<void>;
}

export const RenameModal: React.FC<RenameModalProps> = ({
  isOpen,
  initialName,
  onClose,
  onSave,
}) => {
  const [name, setName] = useState(initialName);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setName(initialName);
    setError(null);
  }, [initialName, isOpen]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;

    setIsLoading(true);
    setError(null);
    try {
      await onSave(name.trim());
      onClose();
    } catch (err: any) {
      // 03-web.md §1.10.4: Display inline form error, do not close modal, keep input
      setError('変更を保存できませんでした。通信状態を確認してもう一度お試しください。');
    } finally {
      setIsLoading(false);
    }
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} align="left">
      <h4>名前の変更</h4>
      {error && <FormError>{error}</FormError>}
      <form onSubmit={handleSubmit} data-testid="rename-form">
        <div className="field">
          <label htmlFor="device-rename-input">端末名</label>
          <input
            id="device-rename-input"
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            disabled={isLoading}
            data-testid="rename-input"
          />
        </div>
        <div className="actions">
          <Button type="button" onClick={onClose} disabled={isLoading}>
            キャンセル
          </Button>
          <Button
            type="submit"
            variant="primary"
            disabled={isLoading || !name.trim() || name.trim() === initialName}
            data-testid="rename-submit-btn"
          >
            {isLoading ? '保存中...' : '再試行'}
          </Button>
        </div>
      </form>
    </Modal>
  );
};
