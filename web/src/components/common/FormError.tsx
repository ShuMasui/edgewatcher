import React from 'react';

interface FormErrorProps {
  children: React.ReactNode;
}

export const FormError: React.FC<FormErrorProps> = ({ children }) => {
  return (
    <div className="form-error" role="alert" data-testid="form-error">
      <span className="label">エラー</span>
      <div>{children}</div>
    </div>
  );
};
