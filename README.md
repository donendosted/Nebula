<img width="234" height="62" alt="image" src="https://github.com/user-attachments/assets/28d0de1f-3679-4e2c-9a0f-7a1df46ca93f" />

# Nebula

Nebula is a local-first, security-focused Stellar wallet for operators, developers, and power users.

- Quick navigation:
  - [Metrics Dashboard](#metrics-dashboard)
  - [Monitoring Dashboard](#monitoring-dashboard)
  - [Security](#security)
  - [Advanced Feature](#advanced-feature)
  - [Data Indexing](#data-indexing)
  - [Community Contribution](#community-contribution)
  - [User Feedback](#user-feedback)

- What this project demonstrates:
  - encrypted HD wallet storage
  - Stellar multisig proposal, signing, and submission flow
  - local transaction indexing and analytics
  - CLI + TUI backed by shared Go packages
  - local observability via Prometheus-compatible metrics

## Why This Matters On Stellar

Stellar accounts are operationally sensitive: signer weights, thresholds, reserves, sequence numbers, and network separation directly affect whether funds remain safe and whether transactions succeed. Nebula focuses on those realities with encrypted local custody, multisig transaction flow, reserve-aware sending, account reloading before submission, and local indexing for fast operator visibility.

- `nb`: scriptable Cobra CLI
- `nbtui`: Bubble Tea terminal UI
- `wallet/`: encrypted HD wallet storage and derivation
- `stellar/`: Horizon and transaction adapter
- `multisig/`: Stellar signer, threshold, and proposal flow
- `indexer/`: local transaction cache and analytics
- `internal/metrics/`: shared observability counters, summaries, and local `/metrics` server
- `internal/monitoring/`: monitoring URLs and browser-open helpers
- `internal/tui/`: Bubble Tea monitoring view helpers
- `nebula/`: compatibility SDK used by the original wallet flows

## Download And Use

### From GitHub Releases

Download the archive for your platform from GitHub Releases, extract it, and run the binaries directly:

- `nebula-linux-amd64.tar.gz`
- `nebula-linux-arm64.tar.gz`
- `nebula-darwin-amd64.tar.gz`
- `nebula-darwin-arm64.tar.gz`
- `nebula-windows-amd64.zip`

```bash
./nb --help
./nbtui
```

Windows:

```powershell
.\nb.exe --help
.\nbtui.exe
```

Fix for apple security bypass : 
```# macOS fix
xattr -d com.apple.quarantine nb nbtui
```

To install globally:

Linux/macOS:

```bash
chmod +x nb nbtui
sudo mv nb /usr/local/bin/
sudo mv nbtui /usr/local/bin/
```

Windows:

1. Move `nb.exe` and `nbtui.exe` into a tools directory.
2. Add that directory to your `PATH`.

### Build From Source

```bash
git clone https://github.com/donendosted/Nebula.git
cd Nebula 
go build -o nb ./cmd/nb
go build -o nbtui ./cmd/nbtui
```

Run locally:

```bash
./nb man
./nbtui
```

Install globally from source:

```bash
go install ./cmd/nb ./cmd/nbtui
```

## Storage

Nebula stores wallet and index data in `~/.nebula/`:

- `wallet.db/`: encrypted HD wallet BadgerDB
- `index.db/`: local transaction index BadgerDB
- `proposals/`: multisig proposal JSON files

Mnemonic material is encrypted with AES-256-GCM and scrypt-derived keys. Nebula does not persist plaintext Stellar seeds.

## Architecture

Nebula keeps the CLI and TUI thin. Core wallet, multisig, indexing, and Stellar network logic live in shared packages:

```text
cmd/nb
  -> wallet/     encrypted mnemonic storage, derivation, active account state
  -> stellar/    Horizon account reload, XDR signing, submission, retry handling
  -> multisig/   signer changes, threshold safety checks, proposal/sign/submit flow
  -> indexer/    local transaction cache, search, and aggregate stats
  -> internal/metrics     shared counters, latency histograms, recent actions
  -> internal/monitoring  local monitoring/open-browser helpers

cmd/nbtui
  -> wallet/     active account login, create/import, wallet switching
  -> stellar/    balance refresh, live send, Friendbot funding
  -> multisig/   proposal creation for multi-party signing
  -> indexer/    indexed history view backed by local cache
  -> internal/metrics     shared monitoring snapshot
  -> internal/tui         monitoring screen renderer
```

Transaction flow:

1. Unlock encrypted mnemonic with a passphrase.
2. Derive the active SEP-0005 account.
3. Reload the Stellar account before building a transaction.
4. Build payment or set-options XDR.
5. Sign directly or store proposal XDR for multi-party signing.
6. Submit and optionally sync into the local index.

## CLI

Core command groups are shown below. The CLI is designed for direct, scriptable use rather than interactive prompts.

HD wallet and account management:

```bash
nb wallet create --wallet-name ops-root --words 24
nb wallet import "abandon ... zoo" --wallet-name recovery-root
nb wallet derive ops-root --index 2 --account-name hot-2
nb wallet list
nb wallet switch ops-root --account-index 2
nb wallet address
```

Multisig operations:

```bash
nb account add-signer G... --weight 1 --yes
nb account remove-signer G... --yes
nb account set-threshold --low 1 --medium 2 --high 2 --master 1 --yes
nb tx propose G... 10 --memo "ops payout"
nb tx sign ops-root-2-1714478000
nb tx submit ops-root-2-1714478000
```

Index and analytics:

```bash
nb index sync
nb search --account G... --since 24h
nb stats --account G...
```

Standard wallet operations remain available:

```bash
nb balance
nb send G... 1.5 --memo "manual payment"
nb history
nb network set mainnet
nb man
```

Observability commands:

```bash
nb stats
nb monitor --open=true
```

## SDK And GoDoc

The core packages are documented for GoDoc and can be used directly by other Go programs:

```go
import (
	"nebula/indexer"
	"nebula/multisig"
	"nebula/stellar"
	"nebula/wallet"
)
```

### `wallet`

`wallet.Store` manages encrypted mnemonic roots in `~/.nebula/wallet.db`.

```go
store, err := wallet.NewStore()
summary, mnemonic, err := store.CreateWallet(wallet.CreateOptions{
	Name:       "ops-root",
	Passphrase: "strong passphrase",
	Words:      24,
})
account, err := store.Derive(summary.ID, "strong passphrase", 1, "hot-1")
_, active, secret, err := store.ActiveAccount("strong passphrase")
```

### `stellar`

`stellar.Client` owns Horizon access, retries, unsigned transaction building, XDR signing, and submission.

```go
client, err := stellar.NewClient("testnet")
acct, err := client.Account(active.Address)
tx, err := client.PaymentTx(acct, "GDEST...", "5.0000000", "ops")
signedXDR, signer, err := client.SignXDR(txe, secret)
```

### `multisig`

`multisig.Service` implements Stellar signer and threshold management plus offline-signable proposal files.

```go
service := multisig.NewService(store)
proposal, err := service.ProposePayment(secret, "testnet", summary.ID, active.Index, "GDEST...", "5.0000000", "ops")
proposal, err = service.SignProposal(secret, proposal.ID)
hash, err := service.SubmitProposal(proposal.ID)
```

### `indexer`

`indexer.Store` keeps a local Badger-backed cache for fast offline queries.

```go
idx, err := indexer.NewStore()
count, err := idx.SyncAccount(client, active.Address, 200)
records, err := idx.SearchAccount(active.Address, 24*time.Hour)
stats, err := idx.Stats(active.Address)
```

### Local GoDoc

`pkgsite` is the best browser experience for local documentation.

Install it once:

```bash
go install golang.org/x/pkgsite/cmd/pkgsite@latest
```

Then from the repo root run:

```bash
./scripts/pkgsite.sh
```

That opens local package docs for:

- `wallet`
- `stellar`
- `multisig`
- `indexer`
- `internal/metrics`
- `internal/monitoring`
- `internal/tui`

For quick terminal docs, you can still use:

```bash
go doc ./wallet
go doc ./multisig
go doc ./stellar
go doc ./indexer
```

## Observability

Nebula exposes local Prometheus-compatible runtime and wallet metrics:

```text
http://localhost:2112/metrics
```

Primary metrics:

- `nebula_tx_success_total`
- `nebula_tx_failure_total`
- `nebula_tx_latency_seconds`
- `nebula_wallet_actions_total{action="create|send|sign|sync|watch"}`
- `nebula_indexed_tx_total`

Instrumentation points:

- wallet create/import increments `create`
- payment send increments `send`
- multisig `tx sign` increments `sign`
- index sync increments `sync`
- CLI/TUI monitor views increment `watch`
- every submitted transaction observes success/failure and latency
- every index sync refreshes `nebula_indexed_tx_total`

Run Prometheus locally:

```bash
prometheus --config.file=observability/prometheus.yml
```

Keep a Nebula process running so Prometheus has something to scrape:

```bash
nb monitor
```

PromQL you can use directly in Prometheus:

- success rate: `sum(rate(nebula_tx_success_total[$__rate_interval]))`
- failure rate: `sum(rate(nebula_tx_failure_total[$__rate_interval]))`
- p95 latency: `1000 * histogram_quantile(0.95, sum(rate(nebula_tx_latency_seconds_bucket[$__rate_interval])) by (le))`
- indexed tx count: `nebula_indexed_tx_total`

TUI monitoring:

- press `m` to open the monitoring screen
- press `r` to refresh immediately
- press `o` to open the local Prometheus UI
- the screen auto-refreshes every 5 seconds while open
- press `H` or `?` to open the TUI actions help screen

## User Feedback

Fill this table as you collect tester feedback and link the implementation commits that addressed it.

| Name | Email | Wallet Address | Feedback | Commit ID |
|------|-------|----------------|----------|-----------|
| Rupam Ghosh | rupamgh32@gmail.com | GC7XMPOXBDBJMPNQ5SQE2DTGACVSX4RHOUXE2XFF2SLHPDJNFGADTIHW | after creating one more wallet the previous wallet disappeared, please bring back my previous wallet, my testnet money[not hard earned] :') | ad6c495aeed1ff9e21610752889c46d0d3cabee5 |
| Susho | sushobhanp59@gmail.com | GDBMOOICQXCNUTYH7XFZ2XCGR7GYLG5UKHG5VRMWEL3YZ255LXBHMV6L | I am facing some issue while running in macos | 163817cd5b4907a904e79743a2954c08929f5cb7 |
| Aditya  | sahaaditya639@gmail.com | GCIIHXIXJW4VLXMSOAX2QJDTGIROJM7UIZLSFIFULV3C2MDQTFY3NN7C | It would have been great to monitor the transaction like how you proposed while in the idea to me | 5f3587d59885b494a969a40771715df80bb131be |
| Ray | ishanray1.02@gmail.com | GDMMLMGYGSWQOGRC2OSUDZYCHQY7JO6TJV2KQSSMLWOWXSG6LNKIKJY6 | Bro there would be a guide for tui as well | 1d9b460f3a13da7caccd1173c331a4210660fa7c |
| Taps | tapsedtzz@gmail.com | GAR7NJQ6QJJTXDABDQSPCFC6MC6A3XP6VD2Y6CTK4WRCIMDKIZX77QLF | The tui is really great but would love a gui interface | *working on it* | 


## Metrics Dashboard

These charts are driven by Nebula's local metrics endpoint and a local Prometheus instance.

![https://img.shields.io/github/downloads/donendosted/Nebula/v0.2/total](https://img.shields.io/github/downloads/donendosted/Nebula/v0.2/total)


**Local Metrics Endpoint:** `http://localhost:2112/metrics`

**Local Prometheus UI:** `http://localhost:9090`

<img width="652" height="585" alt="image" src="https://github.com/user-attachments/assets/c125c008-3d4f-4bba-a072-d38e222919e6" />

The dashboard uses the same PromQL queries listed in the Observability section above.

## Monitoring Dashboard

Nebula includes a built-in monitoring screen inside `nbtui` for quick local inspection without leaving the terminal.

- open with `m`
- refresh with `r`
- open local Prometheus with `o`
- open TUI actions help with `H` or `?`

<img width="792" height="669" alt="image" src="https://github.com/user-attachments/assets/7a5492e1-b0c0-4a86-8d45-fac799660e6c" />


## Security

Nebula emphasizes local custody, encrypted secret storage, safe transaction validation, and safer multisig account changes.

**Link: completed security checklist**

- Architecture reference: [ARCHITECTURE.md](./ARCHITECTURE.md)

### Implemented

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

### Partially Implemented

- Multi-party approval flow
  - Nebula supports propose/sign/submit, but local quorum verification is incomplete.
  - The Stellar network still enforces the final signer/threshold rules at submit time.
- Local observability
  - Prometheus-style metrics and TUI monitoring exist, but this is operational visibility, not a substitute for security audit logging.
- Encrypted wallet storage
  - Secrets are encrypted at rest, but decrypted material is still present in process memory during active use.


## Advanced Features for Black Belt

### Multi-Signature Transaction Approval

Nebula implements a Stellar-native multi-signature workflow built on signer weights, thresholds, unsigned proposal generation, signature collection, and signed XDR submission.

**Description**

- add or remove signers on an account
- configure low / medium / high thresholds
- create unsigned payment proposals
- collect signatures from multiple parties
- submit signed transactions only after proposal signing

**Proof of Implementation**

- signer and threshold management: [multisig/service.go](./multisig/service.go)
- unsigned proposal creation: [multisig/service.go](./multisig/service.go)
- local signature attachment: [multisig/service.go](./multisig/service.go)
- signed proposal submission: [multisig/service.go](./multisig/service.go)
- safety validation against lockout-prone configs: [multisig/service.go](./multisig/service.go)

### Encrypted HD Wallet

Nebula also implements encrypted HD wallet storage with derived account persistence and active-account switching.

**Proof of Implementation**

- encrypted mnemonic storage: [wallet/crypto.go](./wallet/crypto.go)
- wallet database and account derivation persistence: [wallet/store.go](./wallet/store.go)

## Data Indexing

### Approach Description

Nebula uses a local BadgerDB cache in `~/.nebula/index.db` to store normalized Stellar payment and account-creation activity for fast local search and stats.

Key indexing behavior:

- primary transaction records keyed by account + hash
- account-based secondary index for fast per-wallet history lookups
- time-based secondary index for recent activity queries
- local aggregate stats computed from cached records

This keeps history and analytics queries local instead of repeatedly hitting Horizon.

**Proof of Implementation**

- index storage and secondary indexes: [indexer/store.go](./indexer/store.go)
- Horizon operation normalization: [indexer/ops.go](./indexer/ops.go)

**Endpoint or Dashboard Link (if applicable)**

- Local metrics endpoint: `http://localhost:2112/metrics`
- Local Prometheus UI: `http://localhost:9090`

## Community Contribution

Public project sharing:

[<img width="548" height="700" alt="image" src="https://github.com/user-attachments/assets/020ab201-9b3f-44ec-b772-935b7ba5f733" />](https://x.com/donendosted/status/2049906872879366517?s=20)

## Release Flow

Tagged releases are automated with GitHub Actions and GoReleaser:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Download the latest release binaries from the GitHub Releases page.
