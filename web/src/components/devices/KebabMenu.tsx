import React, { useState, useRef, useEffect } from 'react';

interface KebabMenuProps {
  onRename: () => void;
  onDelete: () => void;
}

export const KebabMenu: React.FC<KebabMenuProps> = ({ onRename, onDelete }) => {
  const [isOpen, setIsOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen) return;

    const handleClickOutside = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setIsOpen(false);
      }
    };

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setIsOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    window.addEventListener('keydown', handleKeyDown);

    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
      window.removeEventListener('keydown', handleKeyDown);
    };
  }, [isOpen]);

  return (
    <div style={{ position: 'relative', display: 'inline-block' }} ref={menuRef}>
      <button
        className="kebab"
        onClick={() => setIsOpen((prev) => !prev)}
        aria-label="操作メニューを開く"
        aria-expanded={isOpen}
        data-testid="kebab-button"
      >
        ⋮
      </button>

      {isOpen && (
        <div className="menu" role="menu" data-testid="kebab-menu">
          <button
            role="menuitem"
            onClick={() => {
              setIsOpen(false);
              onRename();
            }}
            data-testid="menu-rename-btn"
          >
            名前の変更
          </button>
          <button
            role="menuitem"
            className="danger"
            onClick={() => {
              setIsOpen(false);
              onDelete();
            }}
            data-testid="menu-delete-btn"
          >
            この端末を削除
          </button>
        </div>
      )}
    </div>
  );
};
