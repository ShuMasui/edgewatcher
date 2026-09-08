import React from 'react';
import { Sidebar } from './Sidebar';

interface ShellProps {
  children: React.ReactNode;
}

export const Shell: React.FC<ShellProps> = ({ children }) => {
  return (
    <div className="app-layout" data-testid="app-shell">
      <Sidebar />
      <div className="main">{children}</div>
    </div>
  );
};
