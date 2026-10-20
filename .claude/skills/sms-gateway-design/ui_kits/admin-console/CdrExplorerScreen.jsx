(() => {
const { Card, Button, IconButton, Badge, StatusPill, DataTable, Pagination, KeyValueList, SpanBar, Banner, EmptyState, ErrorState, SkeletonRows, Input, Select, Tag, Tabs, Modal, Segmented } = window.DS;

const BODY_STATE = {
  stored_encrypted: { icon: 'ban', title: 'Corps stocké (chiffré)', text: 'Affichage possible avec content:read. Chaque lecture est journalisée.' },
  not_stored: { icon: 'ban', title: 'Corps non stocké', text: 'La politique de contenu de ce client est « off ». Le CDR reste complet.' },
  erased: { icon: 'ban', title: 'Corps effacé', text: 'Clé détruite (crypto-shred). Métadonnées conservées, contenu irrécupérable.' },
  expired: { icon: 'ban', title: 'Corps expiré', text: 'Au-delà de content_retention_days. Purge automatique effectuée.' },
};

function CdrExplorerScreen() {
  const M = window.MOCK;
  const [sel, setSel] = React.useState(M.messages[0].id);
  const [tab, setTab] = React.useState('detail');
  const [revealed, setRevealed] = React.useState(false);
  const [exportOpen, setExportOpen] = React.useState(false);
  const msg = M.messages.find((m) => m.id === sel);
  const bs = BODY_STATE[msg.bodyState];
  return (
    <>
      <Toolbar right={<>
        <Button size="sm" variant="secondary">Vues sauvegardées</Button>
        <Button size="sm" variant="secondary" onClick={() => setExportOpen(true)}>Export CSV</Button>
      </>}>
        <Input size="sm" icon="search" placeholder="MSISDN, ID de message, ID de trace…" style={{ width: 280 }} />
        <Select size="sm" placeholder="Tous les clients" options={['Banque X', 'Assur CI', 'Retail Plus']} />
        <Select size="sm" placeholder="Tous les comptes" options={['smpp-banquex-01', 'smpp-banquex-02']} />
        <Select size="sm" placeholder="Tous les statuts" options={['delivered', 'pending', 'failed', 'throttled']} />
        <Tag onRemove={() => {}}>Groupe · Banques CI</Tag>
        <Tag mono onRemove={() => {}}>11/03 00:00 → 12:05</Tag>
      </Toolbar>
      <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0,1fr) var(--detail-panel-width)', alignItems: 'stretch', minHeight: 0 }}>
        <div style={{ borderRight: '1px solid var(--border-default)', background: 'var(--surface-card)' }}>
          <DataTable dense rows={M.messages} selectedKey={sel} onRowClick={(r) => { setSel(r.id); setRevealed(false); }} columns={[
            { key: 'ts', header: 'Horodatage', mono: true, width: 168 },
            { key: 'dest', header: 'Destination', mono: true, width: 140 },
            { key: 'sender', header: 'Expéditeur', mono: true, width: 96 },
            { key: 'account', header: 'Compte SMPP', muted: true },
            { key: 'connector', header: 'Connecteur', muted: true, width: 100 },
            { key: 'status', header: 'Statut', width: 104, render: (r) => <StatusPill state={r.status} /> },
            { key: 'credits', header: 'Crédits', numeric: true, width: 66 },
          ]} />
          <Pagination range="50 lignes" total="~1,24 M au total" pageSize={50} onPageSizeChange={() => {}} hasNext note="curseur serveur · fraîcheur ~15 s" />
        </div>
        <aside style={{ background: 'var(--surface-card)', display: 'flex', flexDirection: 'column', minWidth: 0 }}>
          <div style={{ padding: 'var(--sp-6) var(--sp-7)', borderBottom: '1px solid var(--border-subtle)', display: 'flex', alignItems: 'center', gap: 'var(--sp-4)' }}>
            <span style={{ font: 'var(--text-card-title)' }}>Message</span>
            <span style={{ font: 'var(--text-data)', color: 'var(--text-muted)' }}>{msg.id}</span>
            <span style={{ marginLeft: 'auto', display: 'flex', gap: 'var(--sp-4)', alignItems: 'center' }}>
              <Button variant="link">Copier le lien</Button>
              <IconButton icon="times" label="Fermer le panneau" size="sm" />
            </span>
          </div>
          <Tabs activeId={tab} onChange={setTab} tabs={[{ id: 'detail', label: 'Détail' }, { id: 'trace', label: 'Trace' }, { id: 'body', label: 'Corps' }]} />
          <div style={{ padding: 'var(--sp-7)', overflow: 'auto', display: 'flex', flexDirection: 'column', gap: 'var(--sp-7)' }}>
            {tab === 'detail' ? (
              <>
                <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--sp-6)' }}>
                  <StatusPill state={msg.status} meta={msg.parts + ' partie'} />
                  <Badge tone="mt">MT</Badge>
                </div>
                <KeyValueList mono items={[
                  { label: 'Client', value: msg.customer },
                  { label: 'Compte SMPP', value: msg.account },
                  { label: 'Expéditeur', value: msg.sender },
                  { label: 'Destination', value: msg.dest },
                  { label: 'Route', value: msg.route },
                  { label: 'Connecteur', value: msg.connector },
                  { label: 'Facturation', value: msg.credits ? 'réservé · 1 crédit MT' : 'non facturé (échec)' },
                  { label: 'ID de trace', value: msg.id },
                ]} />
                {msg.route.startsWith('exact:') ? (
                  <Banner tone="warning" title="Route par numéro exact">
                    Prime sur les scripts et le matching déclaratif ; ne court-circuite que la résolution de route.
                  </Banner>
                ) : null}
              </>
            ) : tab === 'trace' ? (
              <>
                <div>{M.trace.map((s, i) => (
                  <SpanBar key={i} name={s.name} depth={s.depth} startPct={s.start} widthPct={s.w} duration={s.dur} state={s.state} />
                ))}</div>
                <Banner tone="info" icon="info" title="Le corps n'apparaît jamais dans la trace." />
              </>
            ) : msg.bodyState !== 'stored_encrypted' ? (
              <EmptyState inline icon={bs.icon} title={bs.title}>{bs.text}</EmptyState>
            ) : revealed ? (
              <>
                <div style={{ font: 'var(--text-code)', background: 'var(--surface-sunken)', border: '1px solid var(--border-subtle)', borderRadius: 'var(--radius-control)', padding: 'var(--sp-6)' }}>
                  Votre code de validation Banque X est 4 8 2 9 1 7. Ne le communiquez à personne.
                </div>
                <Banner tone="warning" title="Lecture journalisée">
                  Consignée dans le journal d'accès au contenu : A. Kouadio · content:read · {msg.id}.
                </Banner>
              </>
            ) : (
              <EmptyState inline icon="ban" title="Corps stocké (chiffré)"
                action={<Button size="sm" variant="secondary" onClick={() => setRevealed(true)}>Afficher le corps</Button>}>
                Requiert content:read. Afficher déclenche un appel audité — lecture journalisée.
              </EmptyState>
            )}
          </div>
        </aside>
      </div>
      <Modal open={exportOpen} title="Export CSV de masse" onClose={() => setExportOpen(false)}
        footer={<><Button variant="ghost" onClick={() => setExportOpen(false)}>Annuler</Button><Button variant="primary" icon="download" onClick={() => setExportOpen(false)}>Lancer le job</Button></>}>
        <Banner tone="warning" title="Export gouverné">
          Permission cdr:export_bulk · plafond de 500 000 lignes · MSISDN masqués selon le rôle · artefact expirant sous 24 h · action auditée.
        </Banner>
        <KeyValueList items={[
          { label: 'Filtres', value: 'Groupe Banques CI · 11/03 00:00 → 12:05' },
          { label: 'Lignes estimées', value: '≈ 24 380' },
          { label: 'Colonnes de corps', value: 'exclues (content:read non détenu sur l’export)' },
        ]} />
      </Modal>
    </>
  );
}
Object.assign(window, { CdrExplorerScreen });
})();
