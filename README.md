# Nebula

Minimal Stellar wallet with:

- `nb` for scriptable CLI usage
- `nbtui` for a Bubble Tea dashboard
- shared logic in `internal/wallet`

Storage:

- Uses the platform config directory via `os.UserConfigDir()`
- Wallet data is stored under `.../nebula/`
- On Linux this is typically `~/.config/nebula`

CLI flow:

```bash
go build -o nb ./cmd/nb

./nb wallet create
./nb wallet list
./nb wallet switch G...
./nb network
./nb fund
./nb balance
./nb history
./nb send G... 1.5 --memo "test payment"
./nb network set mainnet
```

Build:

```bash
go build ./cmd/nb
go build ./cmd/nbtui
```
