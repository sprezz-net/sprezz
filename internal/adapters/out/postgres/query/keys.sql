-- name: InsertHistoricalKey :exec
-- Archives an actor's public key during a rotation event [source: 2].
INSERT INTO actor_public_key_history (
    actor_iri,
    key_type,
    public_key_pem,
    valid_from,
    valid_to
) VALUES ($1, $2, $3, $4, $5);

-- name: FindHistoricalKeyInWindow :one
-- Looks up which public key was active when an inbound activity was signed.
SELECT public_key_pem
FROM actor_public_key_history
WHERE actor_iri = @actor_iri
  AND key_type = @key_type
  AND valid_from <= @signed_at
  AND valid_to >= @signed_at
LIMIT 1;

-- name: DeleteActorKeyHistory :exec
-- Cleans up historical data if a local actor profile is purged.
DELETE FROM actor_public_key_history
WHERE actor_iri = $1;
