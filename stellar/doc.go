// Package stellar provides Nebula's Horizon and transaction-building client
// layer.
//
// The package centralizes network-specific account loading, transaction
// building, submission, and history retrieval so higher-level packages do not
// depend directly on Horizon client details.
package stellar
