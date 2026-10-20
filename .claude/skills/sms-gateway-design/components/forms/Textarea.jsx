import React from 'react';

export function Textarea({ mono = false, invalid = false, rows = 4, className = '', ...rest }) {
  const cls = ['pl-input', 'pl-textarea', mono ? 'pl-input--mono' : '', invalid ? 'pl-input--invalid' : '', className].filter(Boolean).join(' ');
  return <textarea className={cls} rows={rows} aria-invalid={invalid || undefined} {...rest} />;
}
