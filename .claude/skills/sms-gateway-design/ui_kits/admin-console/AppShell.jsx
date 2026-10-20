(() => {
const NS = window.PasserelleDesignSystem_18220e || {};
const { SideNav, TopBar, Toast, ToastStack } = NS;

/* Le bundle du design system est régénéré à part : si un export manque (composant
   ajouté juste avant compilation), on rend un marqueur inerte plutôt que de faire
   tomber tout l'arbre React. */
function missing(name) {
  const Placeholder = (props) => React.createElement('span', {
    style: { display: 'inline-flex', alignItems: 'center', gap: 6, padding: '2px 8px', borderRadius: 'var(--r-pill)', border: '1px dashed var(--border-default)', font: 'var(--text-data-sm)', color: 'var(--text-faint)' },
  }, name + ' indisponible');
  Placeholder.displayName = 'Missing(' + name + ')';
  return Placeholder;
}
const DS = new Proxy({}, { get: (_, k) => NS[k] || missing(String(k)) });

const NAV_GROUPS = [
  { label: 'Exploitation', items: [
    { id: 'traffic', label: 'Trafic' },
    { id: 'cdr', label: 'CDR Explorer' },
    { id: 'sessions', label: 'Sessions', count: 412 },
    { id: 'connectors', label: 'Connecteurs', count: 4 },
  ] },
  { label: 'Clients', items: [
    { id: 'customers', label: 'Clients', count: 87 },
    { id: 'account', label: 'Comptes SMPP', count: 134 },
    { id: 'groups', label: 'Groupes' },
  ] },
  { label: 'Routage', items: [
    { id: 'routes', label: 'Routes' },
    { id: 'exact', label: 'Numéros exacts' },
    { id: 'scripts', label: 'Scripts' },
  ] },
  { label: 'Conformité', items: [
    { id: 'optout', label: 'Désabonnements' },
    { id: 'content', label: 'Contenu & RGPD' },
    { id: 'audit', label: "Journal d'audit" },
  ] },
  { label: 'Facturation', items: [
    { id: 'billing', label: 'Soldes & crédits' },
    { id: 'plans', label: 'Plans tarifaires' },
  ] },
];

function AppShell({ activeId, onNavigate, crumbs, title, badges, actions, tabs, toasts, onDismissToast, children }) {
  return (
    <div style={{ display: 'flex', height: '100vh', overflow: 'hidden', background: 'var(--surface-page)' }}>
      <SideNav monogram="SG" wordmark="SMS Gateway" env="PROD" groups={NAV_GROUPS} activeId={activeId} onNavigate={onNavigate} />
      <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minWidth: 0 }}>
        <TopBar crumbs={crumbs} title={title} badges={badges} actions={actions} operator="A. Kouadio" role="ops" />
        {tabs}
        <main style={{ flex: 1, overflow: 'auto', minWidth: 0 }}>{children}</main>
      </div>
      {toasts && toasts.length ? (
        <ToastStack>
          {toasts.map((t) => (
            <Toast key={t.id} severity={t.severity} title={t.title} source={t.source} onClose={() => onDismissToast(t.id)}>{t.text}</Toast>
          ))}
        </ToastStack>
      ) : null}
    </div>
  );
}

/* Charte §07 : aire teal + ligne pour la métrique principale, ligne bleue pour la
   secondaire, grille discrète, pas d'axes lourds. */
function Chart({ series, secondary, height = 104, label, primaryColor = 'var(--viz-primary)', areaColor = 'var(--viz-primary-area)' }) {
  const all = secondary ? series.concat(secondary) : series;
  const max = Math.max(...all) * 1.14;
  const path = (arr) => arr.map((v, i) => `${i ? 'L' : 'M'}${((i / (arr.length - 1)) * 100).toFixed(2)},${(100 - (v / max) * 100).toFixed(2)}`).join(' ');
  const line = path(series);
  return (
    <div style={{ position: 'relative', height }} aria-label={label} role="img">
      <svg viewBox="0 0 100 100" preserveAspectRatio="none" style={{ width: '100%', height: '100%', display: 'block' }}>
        <line x1="0" y1="33" x2="100" y2="33" stroke="var(--viz-grid)" strokeWidth="0.4" />
        <line x1="0" y1="66" x2="100" y2="66" stroke="var(--viz-grid)" strokeWidth="0.4" />
        <path d={`${line} L100,100 L0,100 Z`} fill={areaColor} />
        <path d={line} fill="none" stroke={primaryColor} strokeWidth="1.4" vectorEffect="non-scaling-stroke" />
        {secondary ? <path d={path(secondary)} fill="none" stroke="var(--viz-secondary)" strokeWidth="1.2" vectorEffect="non-scaling-stroke" /> : null}
      </svg>
    </div>
  );
}

function Legend({ items }) {
  return (
    <div style={{ display: 'flex', gap: 'var(--sp-6)', alignItems: 'center' }}>
      {items.map((it) => (
        <span key={it.label} style={{ display: 'flex', alignItems: 'center', gap: 'var(--sp-3)' }}>
          <span style={{ width: 8, height: 8, borderRadius: 2, background: it.color }} />
          <span style={{ font: 'var(--text-data-sm)', color: 'var(--text-muted)' }}>{it.label}</span>
        </span>
      ))}
    </div>
  );
}

function Toolbar({ children, right }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--sp-4)', padding: 'var(--sp-4) var(--sp-7)', background: 'var(--surface-page)', borderBottom: '1px solid var(--border-default)', minHeight: 'var(--subbar-height)', flexWrap: 'wrap' }}>
      {children}
      {right ? <div style={{ marginLeft: 'auto', display: 'flex', gap: 'var(--sp-4)', alignItems: 'center' }}>{right}</div> : null}
    </div>
  );
}

function Page({ children, pad = true, columns }) {
  return (
    <div style={{ padding: pad ? 'var(--sp-7)' : 0, display: 'grid', gap: 'var(--sp-9)', gridTemplateColumns: columns || '1fr', alignItems: 'start', maxWidth: 'var(--content-max)' }}>{children}</div>
  );
}

Object.assign(window, { AppShell, Chart, Legend, Toolbar, Page, NAV_GROUPS, DS });
})();
