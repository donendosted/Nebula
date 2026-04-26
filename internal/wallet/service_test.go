package wallet

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stellar/go-stellar-sdk/keypair"
)

func TestParseAmountToStroops(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{input: "1", want: 10_000_000},
		{input: "1.5", want: 15_000_000},
		{input: "0.0000001", want: 1},
		{input: "0", wantErr: true},
		{input: "-1", wantErr: true},
		{input: "1.00000001", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got, err := ParseAmountToStroops(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}

func TestStorageRoundTrip(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)

	storage, err := NewStorage()
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	full, err := keypair.Random()
	if err != nil {
		t.Fatalf("random keypair: %v", err)
	}

	walletData, err := storage.SaveWallet(full.Seed())
	if err != nil {
		t.Fatalf("save wallet: %v", err)
	}

	if walletData.Address == "" {
		t.Fatal("expected wallet address")
	}

	loaded, err := storage.LoadWallet()
	if err != nil {
		t.Fatalf("load wallet: %v", err)
	}

	if loaded.Secret != walletData.Secret {
		t.Fatalf("loaded secret mismatch")
	}

	if err := storage.SaveNetwork(NetworkMainnet); err != nil {
		t.Fatalf("save network: %v", err)
	}

	networkValue, err := storage.LoadNetwork()
	if err != nil {
		t.Fatalf("load network: %v", err)
	}

	if networkValue != NetworkMainnet {
		t.Fatalf("got network %q", networkValue)
	}

	active, err := storage.LoadWallet()
	if err != nil {
		t.Fatalf("load active wallet: %v", err)
	}

	if active.Address != walletData.Address {
		t.Fatalf("got active wallet %q want %q", active.Address, walletData.Address)
	}
}

func TestCorruptWalletFile(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)

	storage, err := NewStorage()
	if err != nil {
		t.Fatalf("new storage: %v", err)
	}

	if err := os.MkdirAll(storage.BaseDir(), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(storage.BaseDir(), secretFileName), []byte("broken"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	_, err = storage.LoadWallet()
	if err != ErrCorruptWallet {
		t.Fatalf("got err %v", err)
	}
}

func TestCurrentNetworkDefaultsToTestnet(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)

	svc, err := NewService(ServiceOptions{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	networkValue, err := svc.CurrentNetwork("")
	if err != nil {
		t.Fatalf("current network: %v", err)
	}

	if networkValue != NetworkTestnet {
		t.Fatalf("got %q want %q", networkValue, NetworkTestnet)
	}
}

func TestToggleNetworkPersists(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)

	svc, err := NewService(ServiceOptions{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	next, err := svc.ToggleNetwork()
	if err != nil {
		t.Fatalf("toggle network: %v", err)
	}

	if next != NetworkMainnet {
		t.Fatalf("got %q want %q", next, NetworkMainnet)
	}

	current, err := svc.CurrentNetwork("")
	if err != nil {
		t.Fatalf("current network: %v", err)
	}

	if current != NetworkMainnet {
		t.Fatalf("got %q want %q", current, NetworkMainnet)
	}
}

func TestListAndSwitchWallets(t *testing.T) {
	tempRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tempRoot)
	t.Setenv("HOME", tempRoot)

	svc, err := NewService(ServiceOptions{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	first, err := keypair.Random()
	if err != nil {
		t.Fatalf("first keypair: %v", err)
	}
	second, err := keypair.Random()
	if err != nil {
		t.Fatalf("second keypair: %v", err)
	}

	firstWallet, err := svc.ImportWallet(first.Seed())
	if err != nil {
		t.Fatalf("import first wallet: %v", err)
	}
	secondWallet, err := svc.ImportWallet(second.Seed())
	if err != nil {
		t.Fatalf("import second wallet: %v", err)
	}

	wallets, err := svc.ListWallets()
	if err != nil {
		t.Fatalf("list wallets: %v", err)
	}

	if len(wallets) != 2 {
		t.Fatalf("got %d wallets want 2", len(wallets))
	}

	activeAddress, err := svc.Address()
	if err != nil {
		t.Fatalf("address: %v", err)
	}

	if activeAddress != secondWallet.Address {
		t.Fatalf("got active %q want %q", activeAddress, secondWallet.Address)
	}

	switched, err := svc.SwitchWallet(firstWallet.Address)
	if err != nil {
		t.Fatalf("switch wallet: %v", err)
	}

	if switched.Address != firstWallet.Address {
		t.Fatalf("got switched %q want %q", switched.Address, firstWallet.Address)
	}

	activeAddress, err = svc.Address()
	if err != nil {
		t.Fatalf("address after switch: %v", err)
	}

	if activeAddress != firstWallet.Address {
		t.Fatalf("got active after switch %q want %q", activeAddress, firstWallet.Address)
	}
}
