package wallet

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateDeriveAndUnlock(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStoreAt(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	summary, mnemonic, err := store.CreateWallet(CreateOptions{
		Name:       "ops-root",
		Passphrase: "correct horse battery staple",
		Words:      12,
	})
	require.NoError(t, err)
	require.NotEmpty(t, mnemonic)
	require.Equal(t, "ops-root", summary.Name)
	require.Len(t, summary.Accounts, 1)

	derived, err := store.Derive(summary.ID, "correct horse battery staple", 2, "ops-2")
	require.NoError(t, err)
	require.Equal(t, uint32(2), derived.Index)
	require.NotEmpty(t, derived.Address)

	require.NoError(t, store.SetActiveWallet(summary.ID, 2))
	activeWallet, account, secret, err := store.ActiveAccount("correct horse battery staple")
	require.NoError(t, err)
	require.Equal(t, summary.ID, activeWallet.ID)
	require.Equal(t, uint32(2), account.Index)
	require.NotEmpty(t, secret)
}

func TestImportWalletRejectsBadMnemonic(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStoreAt(dir)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	_, err = store.ImportWallet(ImportOptions{
		Name:       "bad-root",
		Mnemonic:   "one two three",
		Passphrase: "pw",
	})
	require.ErrorIs(t, err, ErrInvalidMnemonic)
}
