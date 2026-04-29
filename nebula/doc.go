// Package nebula is the compatibility SDK used by the original Nebula CLI and TUI paths.
//
// New production code should prefer the focused packages:
//
//   - wallet for encrypted HD wallet storage and derivation
//   - stellar for Horizon access and transaction building/submission
//   - multisig for Stellar signer and threshold workflows
//   - indexer for local transaction caching and analytics
//
// The package remains documented and buildable so existing integrations are not
// broken while the production-oriented modules evolve.
package nebula
