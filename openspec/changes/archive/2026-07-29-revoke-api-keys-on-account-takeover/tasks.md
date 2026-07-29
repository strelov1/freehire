## 1. Keys die with the account's previous holder

- [x] 1.1 Add failing integration tests in `internal/accounts/credential_revocation_integration_test.go`: a squatted account holding a never-expiring full-scope key keeps zero keys after `LinkOrCreateByEmail` seizes it; a `ResetPassword` leaves zero keys; a bystander account's key is untouched by someone else's seizure.
- [x] 1.2 Add the `WITH revoked_keys AS (DELETE FROM api_keys WHERE user_id = $1)` CTE to `SeizeUnverifiedAccount` and `ResetUserPassword` in `internal/db/queries/users.sql`, documenting why the DELETE is welded to the statement rather than issued as a second call. Qualify the update's predicate as `WHERE users.id = $1` — the CTE puts `api_keys` columns in scope and sqlc rejects the bare `id` as ambiguous. Run `sqlc generate`.
- [x] 1.3 Confirm the existing seizure/link tests stay green: a verified account keeps its password and session generation, an unverified passwordless one is only marked verified.

## 2. An unproven address cannot mint a key

- [x] 2.1 Add a failing integration test `TestCreateAPIKeyRequiresAVerifiedAddress` in `internal/handler/api_keys_integration_test.go`: an unverified account with a valid session cookie gets `403` from `POST /me/api-keys` and persists no row.
- [x] 2.2 Turn `CreateAPIKey` into `INSERT ... SELECT $1, $2, … FROM users WHERE users.id = $1 AND users.email_verified`. Select the literal `$1` rather than `users.id` so sqlc names the parameter after the INSERT target column and `CreateAPIKeyParams.UserID` is unchanged.
- [x] 2.3 Map `pgx.ErrNoRows` from `CreateAPIKey` to `403 "confirm your email address before creating an API key"` in `mintAPIKey`. The caller is authenticated, so this is an authorization refusal, not a missing row.
- [x] 2.4 Update the fixtures that mint keys to seed verified accounts (`internal/handler` `seedAccount`, the agent-inbox and market-coverage seeds, `internal/db` `seedAPIKeyUser`, `TestAPIKeyScope`). Leave `internal/db`'s `seedAccount` unverified — the verification and seizure tests depend on it.

## 3. Verification

- [x] 3.1 `go build ./... && go vet ./... && gofmt -l internal/` clean.
- [x] 3.2 `go test ./...` green.
- [x] 3.3 `go test -tags=integration ./...` green.
