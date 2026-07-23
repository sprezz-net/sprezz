-- name: GetNomadicIdentity :one
SELECT n.guid, n.primary_hub_url, n.master_public_key_pem,
    n.created_at
FROM nomadic_identities n
WHERE n.guid = $1
;

-- name: GetIdentityCloneHubs :many
SELECT hub_url
FROM identity_clones
WHERE identity_guid = $1
ORDER BY hub_url;

-- name: UpsertNomadicIdentity :exec
INSERT INTO nomadic_identities (guid, primary_hub_url, master_public_key_pem, created_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (guid) DO UPDATE SET
    primary_hub_url = EXCLUDED.primary_hub_url,
    master_public_key_pem = EXCLUDED.master_public_key_pem;

-- name: RegisterIdentityClone :exec
INSERT INTO identity_clones (identity_guid, hub_url, is_local, synchronized_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (identity_guid, hub_url) DO UPDATE SET
    is_local = EXCLUDED.is_local,
    synchronized_at = NOW();

-- name: GetActorPrivateKey :one
SELECT private_key_pem
FROM local_actor_credentials
WHERE actor_iri = $1;