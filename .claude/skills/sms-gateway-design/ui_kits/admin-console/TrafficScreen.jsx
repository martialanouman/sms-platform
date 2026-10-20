(() => {
const { Card, Button, Badge, StatusPill, DataTable, MetricTile, Banner, Select, Segmented, IconButton } = window.DS;

function TrafficScreen({ onDrill }) {
  const M = window.MOCK;
  const [win, setWin] = React.useState('5m');
  const [tick, setTick] = React.useState(0);
  React.useEffect(() => { const t = setInterval(() => setTick((n) => n + 1), 2500); return () => clearInterval(t); }, []);
  const jitter = (n) => M.fr(n + ((tick * 37) % 120) - 60);
  return (
    <>
      <Toolbar right={<>
        <Segmented value={win} onChange={setWin} items={[{ value: '5m', label: '5 min' }, { value: '1h', label: '1 h' }, { value: '24h', label: '24 h' }]} ariaLabel="Fenêtre temporelle" />
        <Select size="sm" placeholder="Tous les groupes" options={['Banques CI', 'Assurances', 'Retail', 'Secteur public']} />
        <Button size="sm" variant="secondary">Vue sauvegardée</Button>
      </>}>
        <StatusPill state="up" live label="live · WS connecté" meta={win === '5m' ? 'metrics.traffic' : 'instantané REST pré-agrégé'} />
      </Toolbar>
      <Page>
        <div style={{ display: 'grid', gridTemplateColumns: 'repeat(5,1fr)', gap: 'var(--sp-6)' }}>
          <MetricTile label="MT / s" value={jitter(8123)} tone="mt" live delta="4,2 %" />
          <MetricTile label="MO / s" value={jitter(1987)} tone="mo" live delta="1,1 %" />
          <MetricTile label="Taux de succès" value="99,64" unit="%" live />
          <MetricTile label="Latence p99" value="1,24" unit="s" tone="danger" delta="+38 ms" deltaDirection="down" live />
          <MetricTile label="Sessions actives" value="412" footer={<span>398 utilisateur · 14 SMSC</span>} live />
        </div>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--sp-7)' }}>
          <Card title="Débit MT / MO" subtitle="fenêtre 5 min · flux WS"
            actions={<Legend items={[{ label: 'MT', color: 'var(--viz-primary)' }, { label: 'MO', color: 'var(--viz-secondary)' }]} />}>
            <Chart series={M.mtSeries} secondary={M.moSeries} label="Débit MT et MO" />
          </Card>
          <Card title="Taux d'erreur par connecteur" subtitle="fenêtre 5 min · seuil d'alerte 5 %"
            actions={<Badge tone="warning">seuil 5 %</Badge>}>
            <Chart series={M.moSeries.map((v, i) => 40 + (i % 9) * 6 + (i > 30 ? 90 : 0))} label="Taux d'erreur" primaryColor="var(--amber-500)" areaColor="color-mix(in srgb, var(--amber-500) 16%, transparent)" />
          </Card>
        </div>
        <Banner tone="danger" title="Connecteur wholesale-eu en panne (link down) depuis 2 min"
          actions={<Button size="sm" variant="secondary" icon="refresh">Rebind</Button>}>
          Évalué par Alertmanager, indépendamment du tableau de bord. mtn-ci s'appuie par ailleurs sur le disjoncteur sans auto-reconnexion : link_status « reconnecting » et breaker_state « half_open » restent deux dimensions distinctes.
        </Banner>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--sp-7)' }}>
          <Card title="Par connecteur" flush actions={<Button variant="link" onClick={() => onDrill && onDrill('connectors')}>Tout ouvrir</Button>}>
            <DataTable dense rows={M.connectors} onRowClick={() => onDrill && onDrill('cdr')} columns={[
              { key: 'name', header: 'Connecteur', mono: true },
              { key: 'link', header: 'link_status', width: 132, render: (r) => <StatusPill state={r.link} live meta={r.binds} /> },
              { key: 'breaker', header: 'breaker_state', width: 116, render: (r) => <StatusPill state={r.breaker} /> },
              { key: 'tps', header: 'MT/s', numeric: true, width: 70 },
              { key: 'err', header: 'Erreur', numeric: true, width: 78, render: (r) => <span style={{ color: r.err > 5 ? 'var(--text-danger)' : 'var(--text-success)' }}>{String(r.err).replace('.', ',')} %</span> },
              { key: 'part', header: 'Part', width: 90, render: (r) => <span className="pl-meter"><span className="pl-meter__fill" style={{ width: Math.min(r.tps / 6.5, 100) + '%' }} /></span> },
            ]} />
          </Card>
          <Card title="Par client" subtitle="la ventilation par groupe somme les séries par compte" flush>
            <DataTable dense rows={M.byCustomer} onRowClick={() => onDrill && onDrill('cdr')} columns={[
              { key: 'name', header: 'Client' },
              { key: 'group', header: 'Groupe', muted: true },
              { key: 'mt', header: 'MT / s', numeric: true, render: (r) => M.fr(r.mt) },
              { key: 'mo', header: 'MO / s', numeric: true, render: (r) => <span style={{ color: 'var(--mo-accent)' }}>{M.fr(r.mo)}</span> },
              { key: 'success', header: 'Succès', numeric: true, render: (r) => String(r.success).replace('.', ',') + ' %' },
            ]} />
          </Card>
        </div>
      </Page>
    </>
  );
}
Object.assign(window, { TrafficScreen });
})();
