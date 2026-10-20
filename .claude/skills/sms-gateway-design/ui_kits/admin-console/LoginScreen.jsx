(() => {
const { Card, Button, Field, Input, Banner, Badge, Icon, Tabs, EmptyState } = window.PasserelleDesignSystem_18220e;

function LoginScreen({ onDone }) {
  const [step, setStep] = React.useState('password');
  const [method, setMethod] = React.useState('passkey');
  return (
    <div style={{ minHeight: '100vh', display: 'grid', placeItems: 'center', background: 'var(--surface-page)', padding: 'var(--sp-9)' }}>
      <div style={{ width: 400, display: 'flex', flexDirection: 'column', gap: 'var(--sp-8)' }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 'var(--sp-5)' }}>
          <span style={{ width: 34, height: 34, borderRadius: 7, background: 'linear-gradient(140deg, var(--teal-300), var(--teal-700))', color: 'var(--n-900)', display: 'flex', alignItems: 'center', justifyContent: 'center', fontSize: 13, fontWeight: 700 }}>SG</span>
          <span style={{ display: 'flex', flexDirection: 'column' }}>
            <span style={{ font: 'var(--text-card-title)' }}>SMS Gateway</span>
            <span style={{ font: 'var(--text-data-sm)', color: 'var(--text-faint)' }}>admin &amp; exploitation</span>
          </span>
          <span style={{ marginLeft: 'auto' }}><Badge tone="accent">PROD</Badge></span>
        </div>
        <Card title={step === 'password' ? 'Connexion opérateur' : 'Vérification en deux étapes'}
          subtitle={step === 'password' ? 'Tableau de bord d\'exploitation' : 'MFA requis pour les rôles privilégiés'}>
          {step === 'password' ? (
            <form style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-7)' }} onSubmit={(e) => { e.preventDefault(); setStep('mfa'); }}>
              <Field label="Email" htmlFor="em"><Input id="em" type="email" defaultValue="a.kouadio@exploitation.ci" /></Field>
              <Field label="Mot de passe" htmlFor="pw" hint="Verrouillage temporaire après 5 tentatives."><Input id="pw" type="password" defaultValue="••••••••••••" /></Field>
              <Button type="submit" variant="primary" fullWidth>Continuer</Button>
            </form>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 'var(--sp-7)' }}>
              <Tabs activeId={method} onChange={setMethod} tabs={[{ id: 'passkey', label: 'Passkey' }, { id: 'totp', label: 'Code TOTP' }]} />
              {method === 'passkey' ? (
                <>
                  <EmptyState inline  title="Approuvez sur cet appareil">
                    WebAuthn est privilégié quand l'appareil le supporte. Aucun fournisseur d'identité externe n'est impliqué.
                  </EmptyState>
                  <Button variant="primary" fullWidth onClick={onDone}>Utiliser la passkey</Button>
                </>
              ) : (
                <>
                  <Field label="Code à 6 chiffres" hint="Application authenticator enrôlée le 04/01/2026."><Input mono placeholder="000 000" /></Field>
                  <Button variant="primary" fullWidth onClick={onDone}>Vérifier</Button>
                </>
              )}
              <Button variant="ghost" fullWidth onClick={() => setStep('password')}>Retour</Button>
            </div>
          )}
        </Card>
        <Banner tone="info" title="Les identifiants ne quittent jamais la couche serveur">
          Hachage, enrôlement TOTP et cérémonies WebAuthn sont gérés par le BFF, qui émet sa propre session signée.
        </Banner>
      </div>
    </div>
  );
}
Object.assign(window, { LoginScreen });
})();
