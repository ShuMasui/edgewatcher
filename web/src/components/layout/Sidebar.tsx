import React, { useState } from 'react';
import { NavLink, useLocation } from 'react-router-dom';
import { useAuth } from '../../hooks/use-auth';
import { LogoutPopover } from './LogoutPopover';

export const Sidebar: React.FC = () => {
  const location = useLocation();
  const { user } = useAuth();
  const [isPopoverOpen, setIsPopoverOpen] = useState(false);

  const isDashboardActive = location.pathname === '/' || location.pathname.includes('/history');
  const isDevicesActive = location.pathname === '/devices';

  const avatarInitial = user?.name ? user.name[0].toUpperCase() : user?.email ? user.email[0].toUpperCase() : 'S';

  return (
    <aside className="sidebar" data-testid="sidebar">
      <div className="brand">
        <NavLink to="/" className="mark" aria-label="EdgeWatcher ホーム" />
      </div>

      <NavLink
        to="/"
        className={`nav ${isDashboardActive ? 'active' : ''}`}
        data-testid="nav-dashboard"
      >
        最新画像
      </NavLink>

      <NavLink
        to="/devices"
        className={`nav ${isDevicesActive ? 'active' : ''}`}
        data-testid="nav-devices"
      >
        端末一覧
      </NavLink>

      <div className="spacer" />

      <button
        className="avatar-btn"
        onClick={() => setIsPopoverOpen((prev) => !prev)}
        aria-label="User profile and logout menu"
        data-testid="avatar-button"
      >
        <div className="avatar" title={user?.email || 'pentyan0303@gmail.com'}>
          {user?.picture ? <img src={user.picture} alt="" /> : avatarInitial}
        </div>
      </button>

      <LogoutPopover isOpen={isPopoverOpen} onClose={() => setIsPopoverOpen(false)} />
    </aside>
  );
};
