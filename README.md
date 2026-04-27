# Nebula

Minimal Stellar wallet with:

- `nb` for scriptable CLI usage
- `nbtui` for a Bubble Tea dashboard
- reusable SDK in `nebula/`

Storage:

- Uses the platform config directory via `os.UserConfigDir()`
- Wallet data is stored under `.../nebula/`
- Encrypted wallet files live in `.../nebula/wallets/`
- On Linux this is typically `~/.config/nebula`

CLI flow:

```bash
go build -o nb ./cmd/nb

./nb wallet create --name primary
./nb wallet list
./nb wallet switch primary
./nb network
./nb fund
./nb balance
./nb history
./nb send G... 1.5 --memo "test payment"
./nb network set mainnet
./nb man
```

Build:

```bash
go build ./cmd/nb
go build ./cmd/nbtui
```

Install globally:

```bash
go install ./cmd/nb ./cmd/nbtui
```

Automated releases:

```bash
git add .
git commit -m "release"
git tag v0.1.0
git push origin v0.1.0
```

GitHub Actions will publish platform archives automatically with GoReleaser.

You can also use the helper:

```bash
./scripts/release-tag.sh v0.1.0
```
