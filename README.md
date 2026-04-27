<img width="234" height="62" alt="image" src="https://github.com/user-attachments/assets/28d0de1f-3679-4e2c-9a0f-7a1df46ca93f" />

# Nebula

Nebula is a Go-based Stellar wallet project with:

- `nb`: a scriptable CLI wallet
- `nbtui`: a terminal UI wallet built for general use
- `nebula/`: a reusable Go SDK for encrypted wallet storage and Stellar operations
  
View the SDK documentations [here](#SDK)

## Overview

Nebula is split into three layers:

- `cmd/nb`: Cobra-based CLI commands
- `cmd/nbtui`: Bubble Tea TUI application
- `nebula/`: shared SDK used by both binaries

The frontends do not implement wallet logic directly. Encryption, wallet storage, active-wallet switching, Horizon access, funding, sending, and history all live in the SDK.

## Demo Video

https://github.com/user-attachments/assets/7093f335-85f4-4cff-9d04-3bb5e971d5eb

## User feedback

https://docs.google.com/spreadsheets/d/1fvJI2ZTKbtRnlA1-XOwUkVltRIPhzwVfm7Y_660YLdk/edit?usp=sharing

You can share your feedback too here - https://forms.gle/HUpPBBwP4fGfSoRr8

## Download from GitHub Releases

[TEMPRARY PATCH FOR MAC] Macs donot let the app run suspecting it as a malware. Try this instead
	
	```bask
	# macOS fix
	xattr -d com.apple.quarantine nb nbtui
	```

Steps to download and run -

1. Open the project’s GitHub Releases page.
2. Download the archive for your platform:
   - `nebula-linux-amd64.tar.gz`
   - `nebula-linux-arm64.tar.gz`
   - `nebula-darwin-amd64.tar.gz`
   - `nebula-darwin-arm64.tar.gz`
   - `nebula-windows-amd64.zip`
3. Extract the archive.
4. Run the binaries directly:

	`cd` into the directory where you extracted and -

	```bash
	./nbtui
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
	
## Clone And Build Locally

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

Check for your wallet data in here:

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
import "github.com/donendosted/Nebula/nebula"
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
	"github.com/donendosted/Nebula/nebula"
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
