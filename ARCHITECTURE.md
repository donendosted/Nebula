# Nebula Core Architecture

Nebula is organized around four production-focused subsystems:

- `wallet/`: encrypted HD wallet storage, derivation, and confirmation gates
- `multisig/`: Stellar signer and threshold management plus proposal/sign/submit flow
- `indexer/`: local transaction cache, query engine, and analytics
- `stellar/`: Horizon and transaction-building client layer

The CLI in `cmd/nb` is intentionally thin and should delegate all business logic into these packages.

## Data Flow

```text
        +--------------------+
        |     cmd/nb         |
        | Cobra command tree |
        +---------+----------+
                  |
                  v
        +--------------------+
        |      wallet        |
        | unlock + derive    |
        +---------+----------+
                  |
         +--------+--------+
         |                 |
         v                 v
+----------------+  +----------------+
|    multisig    |  |    stellar     |
| signer config  |  | tx build/send  |
| proposal store |  | Horizon sync    |
+--------+-------+  +--------+-------+
         |                   |
         +---------+---------+
                   |
                   v
          +-------------------+
          |      indexer      |
          | cache + search    |
          | local analytics   |
          +-------------------+
```

## Transaction Lifecycle

```text
unlock wallet
  -> derive source key or signer key
  -> reload source account from Horizon
  -> build transaction envelope
  -> if single-sig: sign and submit
  -> if multi-sig: persist proposal XDR, collect signatures, merge signatures, submit
  -> on success: write transaction record into local index
```

## Multisig Signing Flow

```text
nebula tx propose
  -> build unsigned tx envelope XDR
  -> record required threshold and source account snapshot
  -> save proposal locally

nebula tx sign
  -> load proposal
  -> unlock signer wallet/account
  -> attach decorated signature
  -> persist updated proposal

nebula tx submit
  -> reload account state
  -> verify proposal sequence still valid
  -> submit signed transaction
  -> index the accepted transaction locally
```

## Indexing Flow

```text
nebula index sync
  -> fetch recent Horizon transactions for known accounts
  -> normalize tx metadata
  -> write primary records keyed by hash
  -> write secondary keys for account/timestamp/amount
  -> update sync cursor

nebula search / nebula stats
  -> query local BadgerDB only
  -> return results in sub-100ms when warm
```

## Safety Rules

- Never lower thresholds or master key weight in a way that drops current signing power below the new high threshold.
- Always stage signer and threshold changes as proposals and verify effective post-change signer weight before submission.
- Never persist plaintext mnemonic phrases, seeds, or raw secret keys.
- Clear decrypted key material from long-lived structures whenever practical.
- Re-read source account sequence numbers from Horizon before building any outbound transaction.
