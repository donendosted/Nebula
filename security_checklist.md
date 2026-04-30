# Nebula Security Checklist

This checklist is intended to document what Nebula currently implements, what is only partially covered, and what remains to be added before calling the wallet high-assurance or production-hardened.

## Implemented

- Encrypted HD wallet storage
  - Mnemonics are encrypted at rest with AES-256-GCM.
  - Key derivation uses `scrypt`.
  - Reference: [wallet/crypto.go](/home/dos/project-nebula/codex/wallet/crypto.go:1)
- No plaintext mnemonic persistence
  - The wallet database stores encrypted mnemonic material and metadata, not plaintext Stellar secrets.
  - Reference: [wallet/store.go](/home/dos/project-nebula/codex/wallet/store.go:17)
- Read-only database mode for TUI
  - `nbtui` can open the wallet/index stores in read-only mode to avoid unsafe concurrent writes.
  - Reference: [internal/db/db.go](/home/dos/project-nebula/codex/internal/db/db.go:1)
- Local file permissions on multisig proposal files
  - Proposal files are written with `0600`.
  - Reference: [multisig/service.go](/home/dos/project-nebula/codex/multisig/service.go:222)
- Multisig threshold safety validation
  - Threshold ordering is validated.
  - Signer changes are rejected when they would obviously reduce total signer weight below the high threshold.
  - Reference: [multisig/service.go](/home/dos/project-nebula/codex/multisig/service.go:282)
- Reserve-aware payment validation
  - Sends are checked against minimum reserve logic before submission.
  - Reference: [stellar/client.go](/home/dos/project-nebula/codex/stellar/client.go:112)
- Sequence reloading before transaction submission
  - Account state is reloaded from Horizon before payment/proposal submission to reduce stale-sequence failures.
  - Reference: [stellar/client.go](/home/dos/project-nebula/codex/stellar/client.go:58)
  - Reference: [multisig/service.go](/home/dos/project-nebula/codex/multisig/service.go:180)
- Explicit confirmation gates for sensitive wallet and signer actions
  - Sensitive actions require explicit confirmation instead of silent mutation.
  - Reference: [wallet/store.go](/home/dos/project-nebula/codex/wallet/store.go:344)
  - Reference: [multisig/service.go](/home/dos/project-nebula/codex/multisig/service.go:28)

## Partially Implemented

- Multi-party approval flow
  - Nebula supports propose/sign/submit, but local quorum verification is incomplete.
  - The Stellar network still enforces the final signer/threshold rules at submit time.
- Local observability
  - Prometheus-style metrics and TUI monitoring exist, but this is operational visibility, not a substitute for security audit logging.
- Encrypted wallet storage
  - Secrets are encrypted at rest, but decrypted material is still present in process memory during active use.

## Missing Or Should Be Strengthened

- Proposal integrity protection
  - Proposal JSON files are not tamper-evident.
- Local multisig quorum verification before submit
  - Nebula should verify signer weights locally before attempting network submission.
- Secure import path
  - `wallet import <mnemonic>` via command line risks shell history exposure.
  - Prefer stdin, file descriptor, or hidden prompt import.
- Memory hygiene
  - Decrypted mnemonic/secret zeroization is not comprehensive.
- OS keyring or hardware-backed secret handling
  - No keychain, HSM, or hardware-wallet integration yet.
- Encrypted or signed proposal storage
  - Proposal files are local-only, but currently plaintext JSON.
- Audit log for sensitive actions
  - There is no append-only local security event trail.
- Pre-authorized transaction and HashX signer support
  - Advanced Stellar signer modes are not yet implemented.
- Independent cryptographic review
  - The code has not been externally audited.

## Recommended Next Security Additions

- Add `wallet import --stdin` and disable mnemonic echo on terminal input.
- Add local signer-weight verification before `tx submit`.
- Sign or encrypt proposal files.
- Add OS keyring integration for passphrase handling.
- Add explicit inactivity lock / session timeout behavior in `nbtui`.
- Add a security event log for:
  - wallet create/import
  - wallet switch
  - signer changes
  - threshold changes
  - proposal create/sign/submit
- Add fuzz tests around:
  - proposal loading
  - wallet decryption
  - index record decoding

## Threat Model Summary

- Disk theft
  - Mitigated by encrypted mnemonic storage.
- Local shell history leakage
  - Not fully mitigated while mnemonic import remains CLI-argument based.
- Process memory inspection
  - Partially mitigated only by minimizing plaintext persistence, not by strong zeroization.
- Unsafe multisig reconfiguration
  - Partially mitigated by threshold and total-weight checks.
- DB corruption from concurrent access
  - Mitigated in TUI by shared handles and read-only fallback.

## Evidence Links

- Architecture: [ARCHITECTURE.md](/home/dos/project-nebula/codex/ARCHITECTURE.md:1)
- Wallet crypto: [wallet/crypto.go](/home/dos/project-nebula/codex/wallet/crypto.go:1)
- Wallet store: [wallet/store.go](/home/dos/project-nebula/codex/wallet/store.go:1)
- Multisig service: [multisig/service.go](/home/dos/project-nebula/codex/multisig/service.go:1)
- Stellar client: [stellar/client.go](/home/dos/project-nebula/codex/stellar/client.go:1)
- TUI DB lifecycle: [internal/db/db.go](/home/dos/project-nebula/codex/internal/db/db.go:1)
