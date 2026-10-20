Confirmation and one-shot-secret dialogs: forced disconnect, credential rotation, suppression lift, RGPD erasure, show-secret-once.

```jsx
<Modal title="Rotation de la clé API" onClose={close}
  footer={<><Button variant="ghost" onClick={close}>Annuler</Button><Button variant="danger" icon="key-round">Effectuer la rotation</Button></>}>
  <Banner tone="warning" title="Une rotation sans fenêtre de grâce coupe les binds vivants du client." />
</Modal>
```

Every destructive modal names the consequence and the number of affected live sessions before the confirm button.
