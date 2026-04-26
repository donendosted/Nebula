package nebula

import (
	"os"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
)

func TestEncryptedWalletRoundTrip(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	meta, err := store.CreateWallet("alpha", "hunter2")
	if err != nil {
		t.Fatalf("create wallet: %v", err)
	}
	if meta.Address == "" || meta.SecretPath == "" {
		t.Fatalf("missing wallet metadata: %#v", meta)
	}

	info, err := os.Stat(meta.SecretPath)
	if err != nil {
		t.Fatalf("stat wallet file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("got perms %o want 600", info.Mode().Perm())
	}

	unlocked, err := store.UnlockActiveWallet("hunter2")
	if err != nil {
		t.Fatalf("unlock wallet: %v", err)
	}
	if unlocked.Meta.Address != meta.Address {
		t.Fatalf("got address %q want %q", unlocked.Meta.Address, meta.Address)
	}
}

func TestListAndSwitchWallets(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)

	store, err := NewStore()
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	first, err := keypair.Random()
	if err != nil {
		t.Fatalf("first keypair: %v", err)
	}
	second, err := keypair.Random()
	if err != nil {
		t.Fatalf("second keypair: %v", err)
	}

	firstMeta, err := store.ImportWallet("first", first.Seed(), "pw1")
	if err != nil {
		t.Fatalf("import first: %v", err)
	}
	secondMeta, err := store.ImportWallet("second", second.Seed(), "pw2")
	if err != nil {
		t.Fatalf("import second: %v", err)
	}

	wallets, err := store.ListWallets()
	if err != nil {
		t.Fatalf("list wallets: %v", err)
	}
	if len(wallets) != 2 {
		t.Fatalf("got %d wallets want 2", len(wallets))
	}
	if !wallets[0].Active || wallets[0].Address != secondMeta.Address {
		t.Fatalf("expected active wallet first in list")
	}

	switched, err := store.SwitchActiveWallet(firstMeta.Name)
	if err != nil {
		t.Fatalf("switch wallet: %v", err)
	}
	if switched.Address != firstMeta.Address {
		t.Fatalf("got switched %q want %q", switched.Address, firstMeta.Address)
	}

	active, err := store.ActiveWallet()
	if err != nil {
		t.Fatalf("active wallet: %v", err)
	}
	if active.Address != firstMeta.Address {
		t.Fatalf("got active %q want %q", active.Address, firstMeta.Address)
	}
}
