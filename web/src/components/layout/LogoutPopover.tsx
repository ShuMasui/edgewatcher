import React, { useEffect, useRef } from 'react';
import { useAuth } from '../../hooks/use-auth';

interface LogoutPopoverProps {
  isOpen: boolean;
  onClose: () => void;
}

export const LogoutPopover: React.FC<LogoutPopoverProps> = ({ isOpen, onClose }) => {
  const { user, logout } = useAuth();
  const popoverRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen) return;

    const handleClickOutside = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        onClose();
      }
    };

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    window.addEventListener('keydown', handleKeyDown);

    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen, onClose]);

  if (!isOpen) return null;

  return (
    <div className="popover" ref={popoverRef} data-testid="logout-popover">
      <div className="who">
        <div className="mail">{user?.email || 'pentyan0303@gmail.com'}</div>
      </div>
      <button
        onClick={() => {
          onClose();
          logout();
        }}
        data-testid="logout-button"
      >
        <span>⇥</span> ログアウト
      </button>
    </div>
  );
};
