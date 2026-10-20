(() => {
const { Card, Button, Badge, StatusPill, DataTable, KeyValueList, Banner, Field, Input, Select, Modal, BalanceCard, RadioGroup, EmptyState } = window.PasserelleDesignSystem_18220e;

function BillingScreen({ onToast }) {
  const M = window.MOCK;
  const [topup, setTopup] = React.useState(false);
  const [amount, setAmount] = React.useState('500000');
  const [scopeOpen, setScopeOpen] = React.useState(false);
  return (
    <>
      <Toolbar right={<>
        <Select size="sm" placeholder="Direction : toutes" options={['MT', 'MO']} />
        <Button size="sm" variant="secondary">Plan tarifaire</Button>
        <Button size="sm" variant="primary" icon="plus" onClick={() => setTopup(true)}>Recharger</Button>
      </>}>
        <Badge tone="neutral" appearance="outline">balance_scope · pool partagé</Badge>
        <Button size="sm" variant="ghost" onClick={() => setScopeOpen(true)}>Changer de portée</Button>
      </Toolbar>
      <Page>
        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 'var(--sp-7)' }}>
          <BalanceCard direction="mt" value={504219} meterPct={62} scope="Pool partagé · Banque X"
            note="Un vrai solde, rechargeable. Bloque à zéro en prépayé sans découvert autorisé."
            actions={<Button size="sm" variant="primary" icon="plus" onClick={() => setTopup(true)}>Recharger</Button>} />
          <BalanceCard direction="mo" value={3120} unit="crédits consommés" meterPct={31} scope="Plancher mo_billing_floor : 10 000"
            note="Compteur d'usage postpayé, qui monte. Le MO est toujours remis : un dépassement MO ne bloque jamais les envois MT." />
        </div>
        <Card title="Consommation par compte" subtitle="en pool partagé, le grand livre porte owner_* et account_id" flush>
          <DataTable dense rows={[
            { id: 1, account: 'smpp-banquex-01', mt: 412008, mo: 2480, share: '81,7 %' },
            { id: 2, account: 'smpp-banquex-02', mt: 84211, mo: 640, share: '16,7 %' },
            { id: 3, account: 'smpp-banquex-lab', mt: 8000, mo: 0, share: '1,6 %' },
          ]} columns={[
            { key: 'account', header: 'Compte SMPP', mono: true },
            { key: 'mt', header: 'MT consommé', numeric: true, render: (r) => M.fr(r.mt) },
            { key: 'mo', header: 'MO accumulé', numeric: true, render: (r) => M.fr(r.mo) },
            { key: 'share', header: 'Part', numeric: true, width: 90 },
          ]} />
        </Card>
        <Card title="Grand livre" subtitle="filtrable par direction et par compte" flush>
          <DataTable dense rows={M.ledger} rowKey={(r, i) => i} columns={[
            { key: 'ts', header: 'Horodatage', mono: true, width: 120 },
            { key: 'dir', header: 'Direction', width: 90, render: (r) => <Badge tone={r.dir}>{r.dir.toUpperCase()}</Badge> },
            { key: 'label', header: 'Écriture' },
            { key: 'delta', header: 'Delta', numeric: true, width: 110, render: (r) => <span style={{ color: r.delta < 0 ? 'var(--text-danger)' : 'var(--text-success)' }}>{(r.delta > 0 ? '+' : '') + M.fr(r.delta)}</span> },
            { key: 'balance', header: 'Solde après', numeric: true, width: 110, render: (r) => M.fr(r.balance) },
            { key: 'op', header: 'Opérateur', muted: true, width: 110 },
          ]} />
        </Card>
        <Card title="Fournisseur externe" subtitle="proxy fin — le tableau de bord ne stocke aucune donnée financière">
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(3,1fr)', gap: 'var(--sp-7)', alignItems: 'end' }}>
            <Field label="Fournisseur"><Select options={['Interne (crédits)', 'Stripe', 'Wave', 'Orange Money']} /></Field>
            <Field label="Clé publique"><Input mono defaultValue="pk_live_•••• 4A21" /></Field>
            <Button variant="secondary">Tester la connexion</Button>
          </div>
        </Card>
      </Page>
      <Modal open={topup} title="Recharger le solde" onClose={() => setTopup(false)}
        footer={<><Button variant="ghost" onClick={() => setTopup(false)}>Annuler</Button>
          <Button variant="primary" icon="plus" onClick={() => { setTopup(false); onToast && onToast({ severity: 'success', title: 'Recharge de ' + M.fr(Number(amount)) + ' crédits MT', text: 'billing.topup · journalisé', source: 'audit_log' }); }}>Créditer</Button></>}>
        <Field label="Direction" hint="Le MO est un compteur : il ne se recharge pas.">
          <RadioGroup name="dir" value="mt" row options={[{ value: 'mt', label: 'MT (solde)' }, { value: 'mo', label: 'MO (compteur)', disabled: true }]} />
        </Field>
        <Field label="Crédits" hint="Entier non négatif. Chaque nombre est un compteur entier de crédits SMS.">
          <Input mono value={amount} onChange={(e) => setAmount(e.target.value)} />
        </Field>
        <Field label="Compte destinataire" hint="En pool partagé, la recharge crédite le client."><Select options={['Client (pool partagé)', 'smpp-banquex-01', 'smpp-banquex-02']} /></Field>
      </Modal>
      <Modal open={scopeOpen} title="Changer balance_scope" onClose={() => setScopeOpen(false)}
        footer={<><Button variant="ghost" onClick={() => setScopeOpen(false)}>Fermer</Button><Button variant="primary" disabled>Appliquer</Button></>}>
        <Banner tone="warning" title="Action indisponible : tous les soldes doivent être à zéro">
          Le solde MT est de 504 219 crédits. Videz ou transférez les soldes avant de basculer vers « par compte ». Permission requise : billing:scope_change.
        </Banner>
        <RadioGroup name="scope" value="pool" options={[
          { value: 'pool', label: 'Pool partagé', description: 'Un solde pour le client ; ventilation de la consommation par compte.' },
          { value: 'account', label: 'Par compte', description: 'Une carte MT + MO par compte SMPP.', disabled: true },
        ]} />
      </Modal>
    </>
  );
}
Object.assign(window, { BillingScreen });
})();
