import React from 'react';
import { useNavigate } from 'react-router-dom';
import { InlineError } from '../components/common/InlineError';
import { Button } from '../components/common/Button';
import { Shell } from '../components/layout/Shell';

export const NotFoundPage: React.FC = () => {
  const navigate = useNavigate();

  return (
    <Shell>
      <div className="topbar">
        <h3>ページが見つかりません</h3>
      </div>
      <div className="content">
        <InlineError
          mark="404"
          title="ページが見つかりませんでした"
          message="アクセスしようとしたページは存在しないか、移動した可能性があります。"
          actionButton={
            <Button variant="primary" onClick={() => navigate('/')} data-testid="not-found-home-btn">
              ダッシュボードへ戻る
            </Button>
          }
        />
      </div>
    </Shell>
  );
};
