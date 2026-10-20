// Fake data for the Passerelle admin console UI kit. Shapes follow the BFF API in
// uploads/specification-technique-tableau-de-bord.md (§5.1) — values are invented.
window.MOCK = (() => {
  const series = (base, spread, n = 48) => Array.from({ length: n }, (_, i) =>
    Math.round(base + Math.sin(i / 3.1) * spread * 0.6 + Math.cos(i / 1.7) * spread * 0.3 + (i % 7) * spread * 0.05));
  const fr = (n) => n.toLocaleString('fr-FR').replace(/\u202f|\u00a0/g, ' ');
  return {
    fr,
    mtSeries: series(8100, 900),
    moSeries: series(1980, 320),
    connectors: [
      { id: 'orange-ci', name: 'orange-ci', link: 'up', breaker: 'closed', binds: '4/4', tps: 620, err: 0.4, autoReconnect: true },
      { id: 'mtn-ci', name: 'mtn-ci', link: 'reconnecting', breaker: 'half_open', binds: '3/4', tps: 410, err: 5.8, autoReconnect: false },
      { id: 'moov-ci', name: 'moov-ci', link: 'up', breaker: 'closed', binds: '2/2', tps: 180, err: 0.1, autoReconnect: true },
      { id: 'wholesale-eu', name: 'wholesale-eu', link: 'down', breaker: 'open', binds: '0/2', tps: 0, err: 100, autoReconnect: true },
    ],
    byCustomer: [
      { id: 'banquex', name: 'Banque X', group: 'Banques CI', mt: 4820, mo: 1210, success: 99.7 },
      { id: 'assurci', name: 'Assur CI', group: 'Assurances', mt: 1940, mo: 402, success: 99.1 },
      { id: 'retailplus', name: 'Retail Plus', group: 'Retail', mt: 1010, mo: 288, success: 97.2 },
      { id: 'govci', name: 'Services publics', group: 'Secteur public', mt: 353, mo: 87, success: 99.9 },
    ],
    messages: Array.from({ length: 14 }, (_, i) => {
      const st = ['delivered', 'delivered', 'delivered', 'pending', 'failed', 'delivered', 'throttled'][i % 7];
      return {
        id: '01J8ZQ3K7M4YF' + (30 + i).toString(36).toUpperCase(),
        ts: '2026-03-11 12:0' + (4 - (i % 5)) + ':' + (59 - i * 3) + '.' + (100 + i * 7),
        dest: '+2250' + (700000000 + i * 137911),
        sender: ['BANQUEX', 'BANQUEX', 'ALERTE', 'ASSURCI', 'RETAIL+'][i % 5],
        customer: ['Banque X', 'Banque X', 'Banque X', 'Assur CI', 'Retail Plus'][i % 5],
        account: ['smpp-banquex-01', 'smpp-banquex-01', 'smpp-banquex-02', 'smpp-assur-01', 'smpp-retail-01'][i % 5],
        connector: ['orange-ci', 'mtn-ci', 'orange-ci', 'moov-ci', 'orange-ci'][i % 5],
        route: ['CI-mobile-priorité', 'CI-mobile-repli', 'exact:+2250701020304', 'CI-mobile-priorité', 'CI-mobile-priorité'][i % 5],
        status: st,
        credits: st === 'failed' ? 0 : 1,
        parts: 1,
        bodyState: i % 4 === 0 ? 'stored_encrypted' : i % 4 === 1 ? 'not_stored' : i % 4 === 2 ? 'erased' : 'expired',
      };
    }),
    trace: [
      { name: 'ingestion (SMPP submit_sm)', start: 0, w: 6, dur: '4 ms', state: 'ok' },
      { name: 'autorisation expéditeur', depth: 1, start: 6, w: 4, dur: '3 ms', state: 'ok' },
      { name: 'opt-out (canal 36000)', depth: 1, start: 10, w: 3, dur: '2 ms', state: 'ok' },
      { name: 'anti-spam', depth: 1, start: 13, w: 5, dur: '4 ms', state: 'ok' },
      { name: 'routage déclaratif', depth: 1, start: 18, w: 7, dur: '6 ms', state: 'ok' },
      { name: 'contrôle de débit', depth: 1, start: 25, w: 3, dur: '2 ms', state: 'ok' },
      { name: 'facturation (réservation)', depth: 1, start: 28, w: 24, dur: '61 ms', state: 'slow' },
      { name: 'envoi SMSC orange-ci', start: 52, w: 33, dur: '94 ms', state: 'ok' },
      { name: 'DLR reçu', start: 85, w: 8, dur: '1,2 s', state: 'ok' },
      { name: 'remise finale', start: 93, w: 7, dur: 'remis', state: 'ok' },
    ],
    sessions: Array.from({ length: 9 }, (_, i) => ({
      id: 'sess-' + (1000 + i),
      bind: ['trx', 'tx', 'rx'][i % 3],
      account: ['smpp-banquex-01', 'smpp-banquex-01', 'smpp-assur-01', 'smpp-retail-01'][i % 4],
      ip: '10.4.' + (12 + i) + '.' + (40 + i * 3),
      since: ['4 j 02 h', '4 j 02 h', '11 h 40', '2 h 05', '38 min'][i % 5],
      tps: 40 + i * 7,
    })),
    routes: [
      { id: 'r1', prio: 1, name: 'CI-mobile-priorité', cond: 'dest ~ ^\\+2250(1|7)', strategy: 'weighted', targets: 'orange-ci 70 · mtn-ci 30', fallback: 'CI-mobile-repli' },
      { id: 'r2', prio: 2, name: 'CI-mobile-repli', cond: 'dest ~ ^\\+2250', strategy: 'failover_priority', targets: 'moov-ci → wholesale-eu', fallback: '—' },
      { id: 'r3', prio: 3, name: 'International', cond: 'dest !~ ^\\+225', strategy: 'least_cost', targets: 'wholesale-eu', fallback: '—' },
      { id: 'r4', prio: 4, name: 'Tests internes', cond: 'account = smpp-lab-01', strategy: 'round_robin', targets: 'moov-ci', fallback: '—' },
    ],
    ledger: [
      { ts: '11/03 09:12', dir: 'mt', label: 'Recharge · virement', delta: 500000, balance: 512450, op: 'M. Diarra' },
      { ts: '11/03 12:04', dir: 'mt', label: 'Consommation horaire', delta: -8231, balance: 504219, op: 'système' },
      { ts: '11/03 12:04', dir: 'mo', label: 'MO reçus · 36000', delta: 1210, balance: 3120, op: 'système' },
      { ts: '10/03 18:40', dir: 'mt', label: 'Transfert vers smpp-banquex-02', delta: -20000, balance: 484219, op: 'M. Diarra' },
    ],
  };
})();
