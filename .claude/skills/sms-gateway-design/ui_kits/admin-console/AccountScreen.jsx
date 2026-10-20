(() => {
const { Card, Button, IconButton, Badge, StatusPill, DataTable, KeyValueList, Banner, Field, Input, Select, Switch, Checkbox, MaskedSecret, Modal, Tabs, EmptyState } = window.PasserelleDesignSystem_18220e;

function AccountScreen({ onToast }) {
  const M = window.MOCK;
  const [tab, setTab] = React.useState('creds');
  const [rotate, setRotate] = React.useState(false);
  const [grace, setGrace] = React.useState('24 heures');
  const [maxSessions, setMaxSessions] = React.useState('4');
  const [disc, setDisc] = React.useState(null);
  const live = M.sessions.filter((s) => s.account === 'smpp-banquex-01');
  const gap = live.length > Number(maxSessions);
  return (
    <>
      <Tabs activeId={tab} onChange={setTab} tabs={[
        { id: 'creds', label: 'Identifiants', icon: 'key-round' },
        { id: 'channels', label: 'Canaux & opérations' },
        { id: 'quotas', label: 'Quotas & sessions', count: live.length },
        { id: 'webhooks', label: 'Webhooks MO/DLR' },
        { id: 'content', label: 'Contenu', icon: 'lock', disabled: true },
      ]} />
      <Page columns={tab === 'creds' ? '1fr 1fr' : '1fr'}>
        {tab === 'creds' ? (
          <>
            <MaskedSecret kind="Identifiant SMPP" last4="X01B" lastUsedAt="il y a 12 s" liveSessions={live.length}
              actions={<><Button size="sm" variant="secondary" icon="refresh" onClick={() => setRotate(true)}>Rotation</Button><Button size="sm" variant="dangerGhost">Révoquer</Button></>} />
            <MaskedSecret kind="Clé API" last4="7F2A" rotationState="grâce jusqu'au 12/03 08:00" lastUsedAt="il y a 2 min" liveSessions={0}
              actions={<><Button size="sm" variant="secondary" icon="refresh" onClick={() => setRotate(true)}>Rotation</Button><Button size="sm" variant="dangerGhost">Révoquer</Button></>} />
            <Card title="Diagnostic d'échec de bind" subtitle="8 dernières heures" flush style={{ gridColumn: '1 / -1' }}>
              <DataTable dense rows={[
                { id: 1, ts: '11/03 09:41:02', ip: '10.4.19.7', reason: 'ESME_RINVPASWD', bind: 'trx' },
                { id: 2, ts: '11/03 09:40:55', ip: '10.4.19.7', reason: 'ESME_RINVPASWD', bind: 'trx' },
                { id: 3, ts: '11/03 06:12:38', ip: '10.4.19.9', reason: 'ESME_RBINDFAIL (max_sessions atteint)', bind: 'rx' },
              ]} columns={[
                { key: 'ts', header: 'Horodatage', mono: true, width: 160 },
                { key: 'ip', header: 'IP source', mono: true, width: 120 },
                { key: 'bind', header: 'Type', mono: true, width: 70 },
                { key: 'reason', header: 'Motif', render: (r) => <span style={{ color: 'var(--text-danger)' }}>{r.reason}</span> },
              ]} />
            </Card>
          </>
        ) : tab === 'channels' ? (
          <Card title="Canaux & opérations SMPP" subtitle="s'appliquent immédiatement">
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--sp-9)' }}>
              <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-6)' }}>
                <Switch label="Canal SMPP activé" defaultChecked />
                <Switch label="Canal REST activé" defaultChecked />
                <Switch label="query_sm autorisé" defaultChecked />
                <Switch label="cancel_sm autorisé" />
              </div>
              <Field label="Politique d'autorisation de sender ID" hint="Les règles de réécriture de la plateforme s'appliquent après cette vérification.">
                <Select options={[{ value: 'strict', label: 'Strict — sender IDs déclarés uniquement' }, { value: 'alpha', label: 'Alphanumériques libres' }, { value: 'any', label: 'Tout autoriser (déconseillé)' }]} />
              </Field>
            </div>
          </Card>
        ) : tab === 'quotas' ? (
          <>
            {gap ? (
              <Banner tone="danger" title={live.length + ' sessions vivantes / limite ' + maxSessions}
                actions={<Button size="sm" variant="danger" icon="ban" onClick={() => setDisc(live[0])}>Forcer la convergence</Button>}>
                Baisser le quota ne coupe pas les binds vivants : la convergence exige une déconnexion explicite.
              </Banner>
            ) : null}
            <div style={{ display: 'grid', gridTemplateColumns: '320px 1fr', gap: 'var(--sp-7)', alignItems: 'start' }}>
              <Card title="Quotas">
                <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-7)' }}>
                  <Field label="max_sessions" hint="Avertissement avant sauvegarde si la valeur est inférieure aux sessions ouvertes.">
                    <Input mono value={maxSessions} onChange={(e) => setMaxSessions(e.target.value)} />
                  </Field>
                  <Field label="Débit maximum (TPS)"><Input mono defaultValue="600" /></Field>
                  <Field label="Quota journalier MT"><Input mono defaultValue="2 000 000" /></Field>
                  <Button variant="primary">Enregistrer</Button>
                </div>
              </Card>
              <Card title="Binds vivants" subtitle={live.length + ' sessions · mises à jour en deltas'} flush
                actions={<StatusPill state="up" live label="sessions.events" />}>
                <DataTable dense rows={live} columns={[
                  { key: 'id', header: 'Session', mono: true, width: 100 },
                  { key: 'bind', header: 'Type', mono: true, width: 64 },
                  { key: 'ip', header: 'IP', mono: true, width: 110 },
                  { key: 'since', header: 'Depuis', muted: true, width: 90 },
                  { key: 'tps', header: 'TPS', numeric: true, width: 60 },
                  { key: 'act', header: '', width: 40, render: (r) => <IconButton icon="ban" label="Déconnecter" size="sm" variant="danger" onClick={(e) => { e.stopPropagation(); setDisc(r); }} /> },
                ]} />
              </Card>
            </div>
          </>
        ) : (
          <Card title="Webhooks MO / DLR" flush actions={<Button size="sm" variant="secondary" icon="plus">Ajouter</Button>}>
            <DataTable dense rows={[
              { id: 1, kind: 'MO', url: 'https://api.banquex.ci/hooks/mo', secret: '•••• 91C4', status: 'active', fails: 0 },
              { id: 2, kind: 'DLR', url: 'https://api.banquex.ci/hooks/dlr', secret: '•••• 4A20', status: 'failed', fails: 37 },
            ]} columns={[
              { key: 'kind', header: 'Type', width: 70, render: (r) => <Badge tone={r.kind === 'MO' ? 'mo' : 'info'}>{r.kind}</Badge> },
              { key: 'url', header: 'URL', mono: true },
              { key: 'secret', header: 'Secret', mono: true, width: 110 },
              { key: 'status', header: 'Statut', width: 100, render: (r) => <StatusPill state={r.status} /> },
              { key: 'fails', header: 'Échecs 24 h', numeric: true, width: 90 },
            ]} />
          </Card>
        )}
      </Page>
      <Modal open={rotate} title="Rotation de l'identifiant" onClose={() => setRotate(false)}
        footer={<><Button variant="ghost" onClick={() => setRotate(false)}>Annuler</Button>
          <Button variant="danger"  onClick={() => { setRotate(false); onToast && onToast({ severity: 'warning', title: 'Rotation effectuée · grâce ' + grace, text: 'Nouveau secret affiché une seule fois.', source: 'audit_log' }); }}>Effectuer la rotation</Button></>}>
        <Banner tone="warning" title={grace === 'aucune' ? 'Une rotation sans fenêtre de grâce coupe immédiatement les ' + live.length + ' binds vivants du client.' : 'L’ancien secret reste accepté pendant la fenêtre de grâce.'} />
        <Field label="Fenêtre de grâce" hint="Rotation manuelle uniquement — aucune rotation automatique n'est planifiée.">
          <Select value={grace} onChange={(e) => setGrace(e.target.value)} options={['aucune', '1 heure', '24 heures', '7 jours']} />
        </Field>
        <Checkbox label="Je comprends que le nouveau secret ne sera affiché qu'une seule fois." />
      </Modal>
      <Modal open={!!disc} title="Déconnexion forcée" onClose={() => setDisc(null)}
        footer={<><Button variant="ghost" onClick={() => setDisc(null)}>Annuler</Button>
          <Button variant="danger" icon="ban" onClick={() => { onToast && onToast({ severity: 'info', title: 'Session ' + (disc && disc.id) + ' déconnectée', text: 'Action journalisée dans le journal d’audit.', source: 'audit_log' }); setDisc(null); }}>Déconnecter</Button></>}>
        <KeyValueList mono items={[
          { label: 'Session', value: disc ? disc.id : '' },
          { label: 'Compte', value: disc ? disc.account : '' },
          { label: 'IP', value: disc ? disc.ip : '' },
        ]} />
        <Banner tone="warning" title="Action journalisée">session.disconnect · A. Kouadio</Banner>
      </Modal>
    </>
  );
}
Object.assign(window, { AccountScreen });
})();
