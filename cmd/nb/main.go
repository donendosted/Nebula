package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"nebula/nebula"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type appState struct {
	networkOverride string
	verbose         bool
}

func main() {
	state := &appState{}
	root := newRootCmd(state)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "nb",
		Short:         "Nebula Stellar wallet CLI",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&state.networkOverride, "network", "", "network to use: testnet|mainnet")
	cmd.PersistentFlags().BoolVar(&state.verbose, "verbose", false, "enable verbose logs")
	cmd.CompletionOptions.DisableDefaultCmd = false
	cmd.InitDefaultCompletionCmd()

	cmd.AddCommand(newWalletCmd(state))
	cmd.AddCommand(newBalanceCmd(state))
	cmd.AddCommand(newSendCmd(state))
	cmd.AddCommand(newHistoryCmd(state))
	cmd.AddCommand(newFundCmd(state))
	cmd.AddCommand(newNetworkCmd(state))
	cmd.AddCommand(newManCmd(state, cmd))

	return cmd
}

func newWalletCmd(state *appState) *cobra.Command {
	var name string

	cmd := &cobra.Command{
		Use:   "wallet",
		Short: "Manage encrypted wallets",
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new encrypted wallet",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			passphrase, err := promptPassphrase("Choose wallet passphrase: ", true)
			if err != nil {
				return err
			}
			result, err := store.CreateWallet(name, passphrase)
			if err != nil {
				return renderError(err)
			}
			fmt.Println(result.Address)
			fmt.Fprintf(os.Stderr, "Created wallet %q and set active\n", result.Name)
			fmt.Fprintf(os.Stderr, "Wallet metadata directory: %s\n", store.WalletsDir())
			fmt.Fprintf(os.Stderr, "Active wallet file: %s\n", result.SecretPath)
			return nil
		},
	}
	createCmd.Flags().StringVar(&name, "name", "", "wallet name")

	importCmd := &cobra.Command{
		Use:   "import <secret>",
		Short: "Import an existing Stellar secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			passphrase, err := promptPassphrase("Choose wallet passphrase: ", true)
			if err != nil {
				return err
			}
			result, err := store.ImportWallet(name, args[0], passphrase)
			if err != nil {
				return renderError(err)
			}
			fmt.Println(result.Address)
			fmt.Fprintf(os.Stderr, "Imported wallet %q and set active\n", result.Name)
			fmt.Fprintf(os.Stderr, "Wallet metadata directory: %s\n", store.WalletsDir())
			fmt.Fprintf(os.Stderr, "Active wallet file: %s\n", result.SecretPath)
			return nil
		},
	}
	importCmd.Flags().StringVar(&name, "name", "", "wallet name")

	addressCmd := &cobra.Command{
		Use:   "address",
		Short: "Print the active wallet address",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			unlocked, _, err := unlockActiveWallet()
			if err != nil {
				return renderError(err)
			}
			fmt.Println(unlocked.Meta.Address)
			return nil
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List saved wallets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			wallets, err := store.ListWallets()
			if err != nil {
				return renderError(err)
			}
			for _, item := range wallets {
				status := "inactive"
				if item.Active {
					status = "active"
				}
				fmt.Printf("%s\t%s\t%s\t%s\n", item.Name, item.Address, status, item.SecretPath)
			}
			fmt.Fprintf(os.Stderr, "Wallet metadata directory: %s\n", store.WalletsDir())
			return nil
		},
	}

	switchCmd := &cobra.Command{
		Use:   "switch <name|address>",
		Short: "Switch the active wallet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			result, err := store.SwitchActiveWallet(args[0])
			if err != nil {
				return renderError(err)
			}
			fmt.Println(result.Address)
			fmt.Fprintf(os.Stderr, "Active wallet switched to %q\n", result.Name)
			fmt.Fprintf(os.Stderr, "Active wallet file: %s\n", result.SecretPath)
			return nil
		},
	}

	cmd.AddCommand(createCmd, importCmd, addressCmd, listCmd, switchCmd)
	return cmd
}

func newBalanceCmd(state *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "Show the active wallet XLM balance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, unlocked, networkValue, err := resolveClient(state)
			if err != nil {
				return renderError(err)
			}
			info, err := client.Balance()
			if err != nil {
				return renderError(err)
			}
			info.Name = unlocked.Meta.Name
			if !info.Funded {
				return fmt.Errorf("Account not funded. Run `nb fund` on testnet or send XLM to %s", info.Address)
			}
			fmt.Printf("%s\t%s\t%s\t%s\n", info.Name, networkValue, nebula.AssetCodeXLM, nativeBalance(info))
			return nil
		},
	}
}

func newSendCmd(state *appState) *cobra.Command {
	var memo string

	cmd := &cobra.Command{
		Use:   "send <address> <amount>",
		Short: "Send XLM from the active wallet",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, _, err := resolveClient(state)
			if err != nil {
				return renderError(err)
			}
			result, err := client.Send(args[0], args[1], memo)
			if err != nil {
				return renderError(err)
			}
			fmt.Printf("%s\t%s\t%s\n", result.Amount, result.AssetCode, result.Hash)
			if strings.TrimSpace(memo) == "" {
				fmt.Fprintln(os.Stderr, `Suggestion: use --memo to add note`)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&memo, "memo", "", "optional text memo")
	return cmd
}

func newHistoryCmd(state *appState) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "history",
		Short: "Show recent transactions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, networkValue, err := resolveClient(state)
			if err != nil {
				return renderError(err)
			}
			history, err := client.History(limit)
			if err != nil {
				return renderError(err)
			}
			for _, entry := range history {
				fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
					entry.Hash,
					entry.Type,
					entry.Direction,
					entry.Amount,
					entry.AssetCode,
					entry.Counterparty,
					entry.ExplorerURL,
				)
			}
			fmt.Fprintf(os.Stderr, "History loaded: %d transaction(s) on %s\n", len(history), networkValue)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", nebula.DefaultHistoryLimit, "number of entries to fetch (max 20)")
	return cmd
}

func newFundCmd(state *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "fund",
		Short: "Fund the active wallet with Friendbot on testnet",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, unlocked, networkValue, err := resolveClient(state)
			if err != nil {
				return renderError(err)
			}
			hash, err := client.FundTestnet()
			if err != nil {
				if errors.Is(err, nebula.ErrFriendbotLimit) {
					fmt.Fprintln(os.Stderr, "Limit reached")
					return nil
				}
				return renderError(err)
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			count, err := store.RecordTestnetFunding(unlocked.Meta.Address)
			if err != nil {
				return err
			}
			fmt.Println(hash)
			if count > 2 {
				count = 2
			}
			fmt.Fprintf(os.Stderr, "Funded %d/2 times on %s\n", count, networkValue)
			return nil
		},
	}
}

func newNetworkCmd(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
		Short: "Show or change the persisted network",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			current, err := resolveNetwork(store, state.networkOverride)
			if err != nil {
				return renderError(err)
			}
			fmt.Println(current)
			return nil
		},
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "set <testnet|mainnet>",
		Short: "Persist the default network",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			networkValue := nebula.Network(strings.ToLower(strings.TrimSpace(args[0])))
			if err := store.SetNetwork(networkValue); err != nil {
				return renderError(err)
			}
			fmt.Println(networkValue)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "toggle",
		Short: "Toggle and persist the default network",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			networkValue, err := store.ToggleNetwork()
			if err != nil {
				return renderError(err)
			}
			fmt.Println(networkValue)
			return nil
		},
	})

	return cmd
}

func newManCmd(state *appState, root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "man",
		Short: "Print the full Nebula command manual",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(renderManual(root))
			return nil
		},
	}
}

func newStore() (*nebula.Store, error) {
	return nebula.NewStore()
}

func resolveClient(state *appState) (*nebula.Client, nebula.UnlockedWallet, nebula.Network, error) {
	unlocked, store, err := unlockActiveWallet()
	if err != nil {
		return nil, nebula.UnlockedWallet{}, "", err
	}
	networkValue, err := resolveNetwork(store, state.networkOverride)
	if err != nil {
		return nil, nebula.UnlockedWallet{}, "", err
	}
	client, err := unlocked.Client(networkValue)
	if err != nil {
		return nil, nebula.UnlockedWallet{}, "", err
	}
	if state.verbose {
		fmt.Fprintf(os.Stderr, "Using wallet %q (%s) on %s\n", unlocked.Meta.Name, unlocked.Meta.Address, networkValue)
	}
	return client, unlocked, networkValue, nil
}

func unlockActiveWallet() (nebula.UnlockedWallet, *nebula.Store, error) {
	store, err := newStore()
	if err != nil {
		return nebula.UnlockedWallet{}, nil, err
	}
	passphrase, err := promptPassphrase("Wallet passphrase: ", false)
	if err != nil {
		return nebula.UnlockedWallet{}, nil, err
	}
	unlocked, err := store.UnlockActiveWallet(passphrase)
	if err != nil {
		return nebula.UnlockedWallet{}, nil, err
	}
	return unlocked, store, nil
}

func resolveNetwork(store *nebula.Store, override string) (nebula.Network, error) {
	if strings.TrimSpace(override) != "" {
		networkValue := nebula.Network(strings.ToLower(strings.TrimSpace(override)))
		if !networkValue.Valid() {
			return "", nebula.ErrUnsupportedNetwork
		}
		return networkValue, nil
	}
	return store.CurrentNetwork()
}

func promptPassphrase(prompt string, confirm bool) (string, error) {
	if value := strings.TrimSpace(os.Getenv("NEBULA_PASSPHRASE")); value != "" {
		return value, nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", fmt.Errorf("no TTY available for passphrase prompt; set NEBULA_PASSPHRASE")
	}
	fmt.Fprint(os.Stderr, prompt)
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}
	if !confirm {
		return string(first), nil
	}
	fmt.Fprint(os.Stderr, "Confirm passphrase: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", fmt.Errorf("read passphrase confirmation: %w", err)
	}
	if !bytes.Equal(first, second) {
		return "", fmt.Errorf("passphrases do not match")
	}
	return string(first), nil
}

func renderManual(root *cobra.Command) string {
	var b strings.Builder
	b.WriteString("Nebula Manual\n\n")
	b.WriteString("Install globally:\n")
	b.WriteString("  go install ./cmd/nb ./cmd/nbtui\n")
	b.WriteString("  or place built binaries in ~/.local/bin and add it to PATH\n\n")
	appendCommandHelp(&b, root, "")
	return b.String()
}

func appendCommandHelp(b *strings.Builder, cmd *cobra.Command, prefix string) {
	if cmd.Hidden {
		return
	}
	path := strings.TrimSpace(prefix + " " + cmd.Name())
	if prefix == "" {
		path = cmd.Name()
	}
	b.WriteString(path + "\n")
	b.WriteString(strings.Repeat("-", len(path)) + "\n")
	if cmd.Short != "" {
		b.WriteString(cmd.Short + "\n")
	}
	if usage := cmd.UseLine(); usage != "" {
		b.WriteString("Usage: " + usage + "\n")
	}
	if cmd.HasAvailableFlags() {
		b.WriteString("\nFlags:\n")
		b.WriteString(strings.TrimSpace(cmd.Flags().FlagUsages()) + "\n")
	}
	b.WriteString("\n")
	for _, child := range cmd.Commands() {
		appendCommandHelp(b, child, path)
	}
}

func nativeBalance(info nebula.AccountInfo) string {
	for _, balance := range info.Balance {
		if balance.AssetCode == nebula.AssetCodeXLM {
			return balance.Amount
		}
	}
	return "0.0000000"
}

func renderError(err error) error {
	switch {
	case errors.Is(err, nebula.ErrInvalidAddress):
		return fmt.Errorf("invalid address")
	case errors.Is(err, nebula.ErrInvalidAmount):
		return fmt.Errorf("amount must be greater than 0")
	case errors.Is(err, nebula.ErrAccountNotFunded):
		return fmt.Errorf("Account not funded. Run `nb fund` first")
	case errors.Is(err, nebula.ErrInsufficientBalance):
		return err
	case errors.Is(err, nebula.ErrInvalidPassphrase):
		return fmt.Errorf("invalid passphrase")
	default:
		return err
	}
}
