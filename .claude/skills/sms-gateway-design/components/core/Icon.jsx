import React from 'react';

/* Charte §07 — « Pas de pictogrammes décoratifs ni d'emoji. Formes géométriques
   simples et glyphes fonctionnels uniquement. » Le jeu ci-dessous EST le jeu
   complet. Un nom inconnu ne rend rien : cela supprime toute icône décorative
   héritée au lieu de la remplacer par une approximation. */
const GLYPHS = {
  dot: () => <circle cx="8" cy="8" r="3.25" fill="currentColor" stroke="none" />,
  square: () => <rect x="4.5" y="4.5" width="7" height="7" rx="1" fill="currentColor" stroke="none" />,
  diamond: () => <path d="M8 2.6 13.4 8 8 13.4 2.6 8Z" />,
  circle: () => <circle cx="8" cy="8" r="5.2" />,
  warning: () => <g><path d="M8 2.8 14.2 13.2H1.8Z" /><path d="M8 6.6v3.1" /><path d="M8 11.4h.01" /></g>,
  bang: () => <g><path d="M8 3.4v6" /><path d="M8 12.2h.01" /></g>,
  info: () => <g><circle cx="8" cy="8" r="5.6" /><path d="M8 7.2v3.6" /><path d="M8 5.2h.01" /></g>,
  plus: () => <g><path d="M8 3.4v9.2" /><path d="M3.4 8h9.2" /></g>,
  minus: () => <path d="M3.4 8h9.2" />,
  times: () => <g><path d="M4 4l8 8" /><path d="M12 4l-8 8" /></g>,
  check: () => <path d="M3.4 8.6 6.4 11.6 12.6 4.9" />,
  'chevron-down': () => <path d="M4 6.4 8 10.4 12 6.4" />,
  'chevron-up': () => <path d="M4 9.6 8 5.6 12 9.6" />,
  'chevron-left': () => <path d="M9.6 4 5.6 8 9.6 12" />,
  'chevron-right': () => <path d="M6.4 4 10.4 8 6.4 12" />,
  'arrow-up': () => <g><path d="M8 12.4V3.6" /><path d="M4.4 7.2 8 3.6l3.6 3.6" /></g>,
  'arrow-down': () => <g><path d="M8 3.6v8.8" /><path d="M4.4 8.8 8 12.4l3.6-3.6" /></g>,
  refresh: () => <g><path d="M13 8a5 5 0 1 1-1.7-3.8" /><path d="M13 2.6v3.2h-3.2" /></g>,
  search: () => <g><circle cx="7.2" cy="7.2" r="3.9" /><path d="M10.2 10.2 13.4 13.4" /></g>,
  ban: () => <g><circle cx="8" cy="8" r="5.4" /><path d="M4.2 11.8 11.8 4.2" /></g>,
  ellipsis: () => <g><circle cx="3.6" cy="8" r=".9" fill="currentColor" stroke="none" /><circle cx="8" cy="8" r=".9" fill="currentColor" stroke="none" /><circle cx="12.4" cy="8" r=".9" fill="currentColor" stroke="none" /></g>,
  'ellipsis-vertical': () => <g><circle cx="8" cy="3.6" r=".9" fill="currentColor" stroke="none" /><circle cx="8" cy="8" r=".9" fill="currentColor" stroke="none" /><circle cx="8" cy="12.4" r=".9" fill="currentColor" stroke="none" /></g>,
  bell: () => <g><path d="M4.4 11.2V7.4a3.6 3.6 0 0 1 7.2 0v3.8l1.2 1.2H3.2Z" /><path d="M6.7 13.8a1.4 1.4 0 0 0 2.6 0" /></g>,
};

const ALIAS = {
  x: 'times', close: 'times', 'x-circle': 'times',
  'check-circle': 'check', 'circle-check': 'check',
  'alert-triangle': 'warning', 'triangle-alert': 'warning',
  'alert-octagon': 'bang', 'alert-circle': 'bang', 'circle-alert': 'bang', 'octagon-alert': 'bang',
  'circle-help': 'info',
  'trending-up': 'arrow-up', 'trending-down': 'arrow-down',
  'rotate-cw': 'refresh', 'refresh-cw': 'refresh', 'rotate-ccw': 'refresh',
  'search-x': 'search', filter: 'chevron-down',
  lock: 'ban', 'eye-off': 'ban', shredder: 'ban', 'file-x': 'ban', 'clock-alert': 'ban',
  power: 'ban', 'trash-2': 'times', 'external-link': 'chevron-right',
  inbox: 'diamond', 'layout-dashboard': 'diamond',
  bookmark: 'square', 'scroll-text': 'square', 'shield-check': 'check',
};

export function Icon({ name, size = 14, strokeWidth = 1.5, className = '', style, title }) {
  const key = GLYPHS[name] ? name : ALIAS[name];
  const draw = key && GLYPHS[key];
  if (!draw) return null;
  return (
    <span
      className={`pl-icon ${className}`.trim()}
      style={{ width: size, height: size, ...style }}
      aria-hidden={title ? undefined : 'true'}
      aria-label={title}
      role={title ? 'img' : undefined}
    >
      <svg width={size} height={size} viewBox="0 0 16 16" fill="none" stroke="currentColor"
        strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round">
        {draw()}
      </svg>
    </span>
  );
}

const DOT_TONE = {
  up: 'var(--status-up)', degraded: 'var(--status-degraded)', down: 'var(--status-down)',
  restricted: 'var(--status-restricted)', accent: 'var(--teal-500)', info: 'var(--sev-info)', idle: 'var(--status-idle)',
};

/** Point de statut nu — le glyphe le plus utilisé de la charte. */
export function Dot({ tone = 'idle', size = 7, live = false, className = '' }) {
  return (
    <span
      className={['pl-status__dot', live ? 'pl-status--live' : '', className].filter(Boolean).join(' ')}
      style={{ width: size, height: size, background: DOT_TONE[tone] || DOT_TONE.idle }}
    />
  );
}
