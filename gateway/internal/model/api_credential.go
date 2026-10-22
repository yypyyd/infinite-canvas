package model

// BuiltinCredentialID identifies 幻图 as the only caller of the account pool.
// It is stored on event logs and used to scope image/video job lookups.
const BuiltinCredentialID = "cred-huantu-builtin"

// APICredential is the in-memory principal attached to in-process requests.
// External API keys were removed; the legacy api_credentials table is
// emptied by migration 000032 and no longer read.
type APICredential struct {
	ID               string
	Name             string
	ConcurrencyLimit int // zero means unlimited
}

// BuiltinCredential returns the principal for 幻图's in-process calls.
func BuiltinCredential() *APICredential {
	return &APICredential{ID: BuiltinCredentialID, Name: "幻图"}
}
