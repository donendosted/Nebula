# Nebula Stellar Wallet

Nebula is a Go-based Stellar wallet project with:

- `nb`: a scriptable CLI wallet
- `nbtui`: a terminal UI wallet built with Bubble Tea
- `nebula/`: a reusable Go SDK for encrypted wallet storage and Stellar operations
  
View the SDK documentations [here](#SDK)

## Overview

Nebula is split into three layers:

- `cmd/nb`: Cobra-based CLI commands
- `cmd/nbtui`: Bubble Tea TUI application
- `nebula/`: shared SDK used by both binaries

The frontends do not implement wallet logic directly. Encryption, wallet storage, active-wallet switching, Horizon access, funding, sending, and history all live in the SDK.

## Download And Use

### Option 1: Download from GitHub Releases

1. Open the project’s GitHub Releases page.
2. Download the archive for your platform:
   - `nebula-linux-amd64.tar.gz`
   - `nebula-linux-arm64.tar.gz`
   - `nebula-darwin-amd64.tar.gz`
   - `nebula-darwin-arm64.tar.gz`
   - `nebula-windows-amd64.zip`
3. Extract the archive.
4. Run the binaries directly:

Linux/macOS:

```bash
./nb
./nbtui
```

Windows:

```powershell
.\nb.exe
.\nbtui.exe
```

5. Install globally if you want them on your `PATH`.

Linux/macOS:

```bash
chmod +x nb nbtui
sudo mv nb /usr/local/bin/
sudo mv nbtui /usr/local/bin/
```

Windows:

1. Move `nb.exe` and `nbtui.exe` into a tools directory.
2. Add that directory to your user `PATH`.

Release archives also include:

- `README.md`
- `install.sh` for Linux/macOS
- `install.ps1` for Windows

### Option 2: Clone And Build From Source

```bash
git clone https://github.com/donendosted/Nebula.git
cd Nebula
go build -o nb ./cmd/nb
go build -o nbtui ./cmd/nbtui
```

Run locally:

```bash
./nb --help
./nbtui
```

Install globally from source:

```bash
go install ./cmd/nb ./cmd/nbtui
```

## Storage

Nebula uses the default platform config directory from `os.UserConfigDir()`.

Examples:

- Linux: `~/.config/nebula`
- macOS: `~/Library/Application Support/nebula`
- Windows: `%AppData%\nebula`

Inside that directory:

- `config.json`: persisted network and active wallet pointer
- `wallets/`: encrypted wallet files

## CLI

Typical flow:

```bash
nb wallet create --name primary
nb wallet list
nb wallet switch primary
nb fund
nb balance
nb history
nb send G... 1.5 --memo "test payment"
nb network set mainnet
nb man
```

Shell completion is available through Cobra:

```bash
nb completion bash
nb completion zsh
nb completion fish
```

## TUI

`nbtui` provides:

- login prompt before entering the wallet
- create wallet flow
- import wallet flow
- send flow
- history view
- settings view
- wallet switching view

Key actions include:

- `s`: send
- `h`: history
- `w`: wallets
- `n`: toggle network
- `y`: copy address
- `q` twice: quit with confirmation

## SDK

The `nebula` package is intended for reuse by other Go programs.

Import:

```go
import "nebula/nebula"
```

### Main Types

- `nebula.Store`: encrypted wallet/config storage
- `nebula.Client`: Stellar client for a decrypted wallet
- `nebula.UnlockedWallet`: active decrypted wallet session

### Wallet Storage API

Create a store:

```go
store, err := nebula.NewStore()
```

Create a wallet:

```go
meta, err := store.CreateWallet("primary", "your-passphrase")
```

Import a wallet:

```go
meta, err := store.ImportWallet("primary", secretSeed, "your-passphrase")
```

List wallets:

```go
wallets, err := store.ListWallets()
```

Switch active wallet:

```go
meta, err := store.SwitchActiveWallet("primary")
```

Unlock the active wallet:

```go
unlocked, err := store.UnlockActiveWallet("your-passphrase")
```

### Stellar Client API

Construct a client directly:

```go
client, err := nebula.NewClient(secretSeed, nebula.NetworkTestnet)
```

Or from an unlocked wallet:

```go
client, err := unlocked.Client(nebula.NetworkTestnet)
```

Available operations:

```go
address := client.Address()
info, err := client.Balance()
result, err := client.Send(destination, "1.5", "memo text")
history, err := client.History(10)
hash, err := client.FundTestnet()
```

### Minimal Example

```go
package main

import (
	"fmt"
	"log"

	"nebula/nebula"
)

func main() {
	store, err := nebula.NewStore()
	if err != nil {
		log.Fatal(err)
	}

	unlocked, err := store.UnlockActiveWallet("your-passphrase")
	if err != nil {
		log.Fatal(err)
	}

	client, err := unlocked.Client(nebula.NetworkTestnet)
	if err != nil {
		log.Fatal(err)
	}

	info, err := client.Balance()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(client.Address(), info.Network, info.Funded)
}
```

## Releases

Releases are automated with GoReleaser and GitHub Actions.

Visit the [releases](https://github.com/donendosted/Nebula/releases)

That pipeline publishes the platform archives to GitHub Releases automatically.
