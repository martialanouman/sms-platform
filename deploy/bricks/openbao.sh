#!/usr/bin/env bash
# Joué à la main, après `bao operator init` et le descellement. Le jeton racine se lit au terminal ; lui et le mot de
# passe d'openbao n'atteignent le pod que par l'entrée standard. Le jeton est révoqué en fin de passage.
set -euo pipefail

password=$(kubectl -n postgres get secret openbao -o 'jsonpath={.data.password}' | base64 -d)
[ -n "$password" ] || { echo "openbao.sh: postgres/openbao est vide" >&2; exit 1; }
read -rsp "Jeton racine : " token
echo >&2
[ -n "$token" ] || { echo "openbao.sh: jeton vide" >&2; exit 1; }

primary=$(kubectl -n postgres get cluster postgres -o 'jsonpath={.status.currentPrimary}')
kubectl -n postgres exec -i "$primary" -c postgres -- psql -v ON_ERROR_STOP=1 -d control_plane <<'SQL'
SET ROLE openbao;
DO $$ BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'breakglass') THEN
    CREATE ROLE breakglass NOLOGIN;
  END IF;
END $$;
SQL

kubectl -n openbao exec -i openbao-0 -c openbao -- sh -c '
set -eu
read -r BAO_TOKEN
read -r pg
export BAO_TOKEN

bao secrets list | grep -q "^transit/" || bao secrets enable transit
bao read transit/keys/config-secrets >/dev/null 2>&1 || bao write -f transit/keys/config-secrets type=aes256-gcm96
bao policy write content-key-svc - <<HCL
path "transit/encrypt/config-secrets" { capabilities = ["update"] }
path "transit/decrypt/config-secrets" { capabilities = ["update"] }
HCL

bao auth list | grep -q "^kubernetes/" || bao auth enable kubernetes
bao write auth/kubernetes/config kubernetes_host="https://$KUBERNETES_SERVICE_HOST:$KUBERNETES_SERVICE_PORT"
bao write auth/kubernetes/role/content-key-svc bound_service_account_names=content-key-svc \
  bound_service_account_namespaces=sms audience=openbao token_ttl=1h token_max_ttl=1h \
  token_policies=content-key-svc token_no_default_policy=true

bao secrets list | grep -q "^database/" || bao secrets enable database
printf "%s" "$pg" | bao write database/config/control_plane plugin_name=postgresql-database-plugin \
  connection_url="postgresql://{{username}}:{{password}}@postgres-rw.postgres.svc:5432/control_plane?sslmode=verify-full&sslrootcert=/openbao/userconfig/openbao-tls/ca.crt" \
  username=openbao password=- allowed_roles=breakglass
bao write database/roles/breakglass db_name=control_plane default_ttl=1h max_ttl=1h \
  creation_statements="CREATE ROLE \"{{name}}\" WITH LOGIN PASSWORD '"'"'{{password}}'"'"' VALID UNTIL '"'"'{{expiration}}'"'"' IN ROLE breakglass;"

bao token revoke -self
' <<<"$token
$password"
echo "openbao.sh: OpenBao configuré, jeton révoqué" >&2
