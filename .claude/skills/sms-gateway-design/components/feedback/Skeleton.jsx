import React from 'react';

export function Skeleton({ width = '100%', height = 10, radius, className = '', style }) {
  return <span className={`pl-skel ${className}`.trim()} style={{ width, height, borderRadius: radius, ...style }} />;
}

export function SkeletonRows({ rows = 3, columns = [220, 90, 60], dense = false, className = '' }) {
  return (
    <div className={className || undefined}>
      {Array.from({ length: rows }, (_, i) => (
        <div className="pl-skelrow" key={i} style={dense ? { padding: '4px 0' } : undefined}>
          {columns.map((w, j) => <Skeleton key={j} width={w} />)}
        </div>
      ))}
    </div>
  );
}
