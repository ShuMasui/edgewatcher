import React from 'react';
import { Button } from './Button';

interface InlineErrorProps {
  title?: string;
  message?: string;
  mark?: string;
  onRetry?: () => void;
  retryLabel?: string;
  actionButton?: React.ReactNode;
}

export const InlineError: React.FC<InlineErrorProps> = ({
  title = '読み込めませんでした',
  message = '通信に失敗しました。しばらく待ってからもう一度お試しください。',
  mark = '!',
  onRetry,
  retryLabel = '再試行',
  actionButton,
}) => {
  return (
    <div className="inline-error" data-testid="inline-error">
      <div className="mark">{mark}</div>
      <h4>{title}</h4>
      <p>{message}</p>
      {actionButton ? (
        actionButton
      ) : onRetry ? (
        <Button variant="primary" onClick={onRetry}>
          {retryLabel}
        </Button>
      ) : null}
    </div>
  );
};
