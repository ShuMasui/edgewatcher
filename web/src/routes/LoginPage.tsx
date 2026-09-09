import React, { useEffect } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useAuth } from '../hooks/use-auth';

export const LoginPage: React.FC = () => {
  const { isAuthenticated, login, error } = useAuth();
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
        <div className="mark lg" />
        <p className="wordmark">EdgeWatcher</p>
        <p>
          余っている Android 端末を、
          <br />
          屋外の定点観測カメラとして使う。
        </p>
        {error && (
          <p className="login-error" role="alert" data-testid="login-error">
            {error}
          </p>
        )}
        <span className="label">サインイン</span>
        <button className="google" onClick={login} data-testid="google-login-btn">
          <svg viewBox="0 0 48 48" aria-hidden="true">
            <path
              fill="#4285f4"
              d="M45.1 24.5c0-1.6-.1-3.1-.4-4.5H24v8.5h11.8c-.5 2.7-2.1 5-4.4 6.6v5.5h7.1c4.2-3.8 6.6-9.5 6.6-16.1z"
            />
            <path
              fill="#34a853"
              d="M24 46c6 0 11-2 14.5-5.4l-7.1-5.5c-2 1.3-4.5 2.1-7.4 2.1-5.7 0-10.6-3.9-12.3-9.1H4.3v5.7C7.8 41.1 15.3 46 24 46z"
            />
            <path
              fill="#fbbc05"
              d="M11.7 28.1c-.4-1.3-.7-2.7-.7-4.1s.3-2.8.7-4.1v-5.7H4.3C2.8 17.2 2 20.5 2 24s.8 6.8 2.3 9.8l7.4-5.7z"
            />
            <path
              fill="#ea4335"
              d="M24 10.8c3.2 0 6.1 1.1 8.4 3.3l6.3-6.3C34.9 4.2 30 2 24 2 15.3 2 7.8 6.9 4.3 14.2l7.4 5.7c1.7-5.2 6.6-9.1 12.3-9.1z"
            />
          </svg>
          Google で続行
        </button>
      </div>
    </div>
  );
};
