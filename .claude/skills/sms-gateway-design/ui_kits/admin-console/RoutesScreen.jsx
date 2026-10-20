(() => {
const { Card, Button, IconButton, Badge, StatusPill, DataTable, KeyValueList, Banner, Field, Input, Select, Modal, Tag, EmptyState } = window.PasserelleDesignSystem_18220e;

function RoutesScreen({ onToast }) {
  const M = window.MOCK;
  const [routes, setRoutes] = React.useState(M.routes);
  const [sel, setSel] = React.useState('r1');
  const [sim, setSim] = React.useState(false);
  const move = (id, dir) => {
    const i = routes.findIndex((r) => r.id === id);
    const j = i + dir;
    if (j < 0 || j >= routes.length) return;
    const next = routes.slice();
    [next[i], next[j]] = [next[j], next[i]];
    setRoutes(next.map((r, k) => ({ ...r, prio: k + 1 })));
  };
  const route = routes.find((r) => r.id === sel);
  return (
    <>
      <Toolbar right={<>
        <Button size="sm" variant="secondary" onClick={() => setSim(true)}>Simuler</Button>
        <Button size="sm" variant="primary" icon="plus">Nouvelle route</Button>
      </>}>
<Badge tone="neutral" appearance="outline">ordre = priorité</Badge>
        <span style={{ font: 'var(--text-data-sm)', color: 'var(--text-faint)' }}>glisser-déposer pour réordonner · routes:write</span>
      </Toolbar>
      <Page columns="minmax(0,1fr) 380px">
        <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-7)' }}>
          <Banner tone="warning" title="Le routage par numéro exact prime sur les scripts et sur ce matching déclaratif">
            Le court-circuit ne saute que la résolution de route : opt-out, autorisation d'expéditeur, anti-spam et facturation continuent de s'appliquer.
          </Banner>
          <Card title="Règles de routage" subtitle={routes.length + ' règles · évaluées de haut en bas'} flush>
            <DataTable rows={routes} selectedKey={sel} onRowClick={(r) => setSel(r.id)} columns={[
              { key: 'prio', header: '#', numeric: true, width: 46 },
              { key: 'name', header: 'Route' },
              { key: 'cond', header: 'Conditions', mono: true },
              { key: 'strategy', header: 'Stratégie', width: 138, render: (r) => <Badge tone="neutral" appearance="outline">{r.strategy}</Badge> },
              { key: 'targets', header: 'Cibles', muted: true },
              { key: 'act', header: '', width: 68, render: (r) => (
                <span style={{ display: 'flex', gap: 2 }}>
                  <IconButton icon="chevron-up" label="Monter" size="sm" onClick={(e) => { e.stopPropagation(); move(r.id, -1); }} />
                  <IconButton icon="chevron-down" label="Descendre" size="sm" onClick={(e) => { e.stopPropagation(); move(r.id, 1); }} />
                </span>
              ) },
            ]} />
          </Card>
          <Card title="Scripts de routage" subtitle="un seul script actif par portée" flush
            actions={<Button size="sm" variant="secondary" >Ouvrir l'éditeur</Button>}>
            <DataTable dense rows={[
              { id: 1, name: 'banquex-otp-priority', scope: 'compte · smpp-banquex-01', v: 'v7', state: 'active', p99: '2,1 ms', err: '0,00 %' },
              { id: 2, name: 'weekend-least-cost', scope: 'plateforme', v: 'v3', state: 'pending', p99: '1,4 ms', err: '0,02 %' },
            ]} columns={[
              { key: 'name', header: 'Script', mono: true },
              { key: 'scope', header: 'Portée', muted: true },
              { key: 'v', header: 'Version', mono: true, width: 70 },
              { key: 'state', header: 'État', width: 110, render: (r) => r.state === 'active' ? <Badge tone="domain">actif</Badge> : <Badge tone="warning">brouillon</Badge> },
              { key: 'p99', header: 'p99', numeric: true, width: 70 },
              { key: 'err', header: 'Erreurs', numeric: true, width: 80 },
            ]} />
          </Card>
        </div>
        <Card title={route.name} subtitle={'priorité ' + route.prio} actions={<Button size="sm" variant="secondary" >Enregistrer</Button>}>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-7)' }}>
            <Field label="Nom"><Input defaultValue={route.name} /></Field>
            <Field label="Condition destination" hint="Testeur regex disponible."><Input mono defaultValue={route.cond} /></Field>
            <Field label="Stratégie de distribution">
              <Select defaultValue={route.strategy} options={['weighted', 'failover_priority', 'least_cost', 'round_robin']} />
            </Field>
            <Field label="Cibles" hint="Poids pour weighted, ordre pour failover_priority.">
              <div style={{ display: 'flex', gap: 'var(--sp-3)', flexWrap: 'wrap' }}>
                <Tag mono onRemove={() => {}}>orange-ci · 70</Tag>
                <Tag mono onRemove={() => {}}>mtn-ci · 30</Tag>
              </div>
            </Field>
            <Field label="Route de repli"><Select defaultValue={route.fallback} options={['—', 'CI-mobile-repli', 'International']} /></Field>
          </div>
        </Card>
      </Page>
      <Modal open={sim} wide title="Simuler un message" onClose={() => setSim(false)}
        footer={<><Button variant="ghost" onClick={() => setSim(false)}>Fermer</Button><Button variant="primary" >Relancer</Button></>}>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--sp-7)' }}>
          <Field label="Compte SMPP"><Select options={['smpp-banquex-01', 'smpp-assur-01']} /></Field>
          <Field label="Expéditeur"><Input mono defaultValue="BANQUEX" /></Field>
          <Field label="Destination"><Input mono defaultValue="+2250701020304" /></Field>
          <Field label="Contenu"><Input defaultValue="Code de validation 482917" /></Field>
        </div>
        <Banner tone="info" title="Ce compte a un script de routage actif (banquex-otp-priority v7)">
          Le script prévaudrait sur le matching déclaratif. Un numéro exact existe aussi pour cette destination — il est prioritaire sur les deux.
        </Banner>
        <KeyValueList mono items={[
          { label: 'Décision', value: 'exact:+2250701020304 → orange-ci' },
          { label: 'Niveau décideur', value: 'routage par numéro exact (plateforme)' },
          { label: 'Route déclarative ignorée', value: 'CI-mobile-priorité' },
        ]} />
      </Modal>
    </>
  );
}
Object.assign(window, { RoutesScreen });
})();
