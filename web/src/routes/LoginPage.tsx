import React, { useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../hooks/use-auth';

export const LoginPage: React.FC = () => {
  const { isAuthenticated, login } = useAuth();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();

  useEffect(() => {
    if (isAuthenticated) {
      const redirect = searchParams.get('redirect') || '/';
      navigate(redirect, { replace: true });
    }
  }, [isAuthenticated, navigate, searchParams]);

  return (
    <div className="login" data-testid="login-page">
      <div className="login-box">
        <div className="logo" />
        <h3>EdgeWatcher</h3>
        <p>
          余っている Android 端末を、
          <br />
          定点観測デバイスとして使う。
        </p>
        <button className="google" onClick={login} data-testid="google-login-btn">
          <span className="gmark" />
          Google で続行
        </button>
      </div>
    </div>
  );
};
