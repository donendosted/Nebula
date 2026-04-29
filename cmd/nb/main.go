package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"nebula/indexer"
	"nebula/multisig"
	"nebula/nebula"
	"nebula/stellar"
	"nebula/wallet"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type appState struct {
	networkOverride string
	verbose         bool
}

type unlockedSession struct {
	Wallet  wallet.WalletSummary
	Account wallet.DerivedAccount
	Secret  string
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

	cmd.AddCommand(newWalletCmd())
	cmd.AddCommand(newAccountCmd(state))
	cmd.AddCommand(newTxCmd(state))
	cmd.AddCommand(newIndexCmd(state))
	cmd.AddCommand(newSearchCmd())
	cmd.AddCommand(newStatsCmd())
	cmd.AddCommand(newBalanceCmd(state))
	cmd.AddCommand(newSendCmd(state))
	cmd.AddCommand(newHistoryCmd(state))
	cmd.AddCommand(newFundCmd(state))
	cmd.AddCommand(newNetworkCmd(state))
	cmd.AddCommand(newManCmd(cmd))

	return cmd
}

func newIndexCmd(state *appState) *cobra.Command {
	var account string
	var limit int

	cmd := &cobra.Command{
		Use:   "index",
		Short: "Manage the local transaction index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	syncCmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync recent transactions into the local index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			idx, err := indexer.NewStore()
			if err != nil {
				return err
			}
			defer idx.Close()
			networkValue, err := resolveNetwork(state.networkOverride)
			if err != nil {
				return renderError(err)
			}
			client, err := stellar.NewClient(string(networkValue))
			if err != nil {
				return err
			}
			targets, err := syncTargets(account)
			if err != nil {
				return renderError(err)
			}
			total := 0
			for _, target := range targets {
				count, err := idx.SyncAccount(client, target, limit)
				if err != nil {
					return err
				}
				total += count
			}
			fmt.Println(total)
			fmt.Fprintf(os.Stderr, "Index DB: %s\n", idx.DBDir())
			return nil
		},
	}
	syncCmd.Flags().StringVar(&account, "account", "", "sync only one account address")
	syncCmd.Flags().IntVar(&limit, "limit", 200, "maximum recent records per account to sync")
	cmd.AddCommand(syncCmd)
	return cmd
}

func newSearchCmd() *cobra.Command {
	var account string
	var since string

	cmd := &cobra.Command{
		Use:   "search",
		Short: "Query the local transaction index",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			idx, err := indexer.NewStore()
			if err != nil {
				return err
			}
			defer idx.Close()
			duration, err := parseSince(since)
			if err != nil {
				return err
			}
			var records []indexer.Record
			if strings.TrimSpace(account) != "" {
				records, err = idx.SearchAccount(account, duration)
			} else {
				records, err = idx.SearchSince(duration)
			}
			if err != nil {
				return err
			}
			for _, record := range records {
				fmt.Printf("%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					record.Timestamp.Format(time.RFC3339),
					record.Account,
					record.Hash,
					record.Direction,
					record.Amount,
					record.AssetCode,
					record.Counterparty,
					record.ExplorerURL,
				)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "filter by account address")
	cmd.Flags().StringVar(&since, "since", "24h", "look back duration")
	return cmd
}

func newStatsCmd() *cobra.Command {
	var account string

	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show local transaction analytics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			idx, err := indexer.NewStore()
			if err != nil {
				return err
			}
			defer idx.Close()
			stats, err := idx.Stats(account)
			if err != nil {
				return err
			}
			fmt.Printf("total_transactions\t%d\n", stats.TotalTransactions)
			fmt.Printf("avg_latency_ms\t%.2f\n", stats.AverageLatencyMS)
			fmt.Printf("total_volume_sent\t%.7f\n", stats.TotalVolumeSent)
			fmt.Printf("total_volume_received\t%.7f\n", stats.TotalVolumeRecv)
			return nil
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "filter by account address")
	return cmd
}

func newAccountCmd(state *appState) *cobra.Command {
	var weight uint8
	var low uint8
	var medium uint8
	var high uint8
	var master uint8
	var yes bool

	cmd := &cobra.Command{
		Use:   "account",
		Short: "Manage Stellar signers and thresholds",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "add-signer <address>",
		Short: "Add or update a signer on the active account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			unlocked, err := unlockActiveWallet()
			if err != nil {
				return renderError(err)
			}
			networkValue, err := resolveNetwork(state.networkOverride)
			if err != nil {
				return renderError(err)
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			service := multisig.NewService(store)
			hash, err := service.AddSigner(unlocked.Secret, string(networkValue), args[0], weight, yes)
			if err != nil {
				return renderError(err)
			}
			fmt.Println(hash)
			return nil
		},
	})
	cmd.Commands()[0].Flags().Uint8Var(&weight, "weight", 1, "signer weight")
	cmd.Commands()[0].Flags().BoolVar(&yes, "yes", false, "confirm sensitive signer change")

	cmd.AddCommand(&cobra.Command{
		Use:   "remove-signer <address>",
		Short: "Remove a signer from the active account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			unlocked, err := unlockActiveWallet()
			if err != nil {
				return renderError(err)
			}
			networkValue, err := resolveNetwork(state.networkOverride)
			if err != nil {
				return renderError(err)
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			service := multisig.NewService(store)
			hash, err := service.RemoveSigner(unlocked.Secret, string(networkValue), args[0], yes)
			if err != nil {
				return renderError(err)
			}
			fmt.Println(hash)
			return nil
		},
	})
	cmd.Commands()[1].Flags().BoolVar(&yes, "yes", false, "confirm sensitive signer change")

	cmd.AddCommand(&cobra.Command{
		Use:   "set-threshold",
		Short: "Set low, medium, high, and master thresholds",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			unlocked, err := unlockActiveWallet()
			if err != nil {
				return renderError(err)
			}
			networkValue, err := resolveNetwork(state.networkOverride)
			if err != nil {
				return renderError(err)
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			service := multisig.NewService(store)
			hash, err := service.SetThresholds(unlocked.Secret, string(networkValue), multisig.ThresholdConfig{
				MasterWeight: master,
				Low:          low,
				Medium:       medium,
				High:         high,
			}, yes)
			if err != nil {
				return renderError(err)
			}
			fmt.Println(hash)
			return nil
		},
	})
	cmd.Commands()[2].Flags().Uint8Var(&low, "low", 1, "low threshold")
	cmd.Commands()[2].Flags().Uint8Var(&medium, "medium", 2, "medium threshold")
	cmd.Commands()[2].Flags().Uint8Var(&high, "high", 2, "high threshold")
	cmd.Commands()[2].Flags().Uint8Var(&master, "master", 1, "master key weight")
	cmd.Commands()[2].Flags().BoolVar(&yes, "yes", false, "confirm sensitive threshold change")

	return cmd
}

func newTxCmd(state *appState) *cobra.Command {
	var memo string
	cmd := &cobra.Command{
		Use:   "tx",
		Short: "Manage multisig transaction proposals",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "propose <address> <amount>",
		Short: "Create a multisig payment proposal",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			unlocked, err := unlockActiveWallet()
			if err != nil {
				return renderError(err)
			}
			networkValue, err := resolveNetwork(state.networkOverride)
			if err != nil {
				return renderError(err)
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			service := multisig.NewService(store)
			proposal, err := service.ProposePayment(unlocked.Secret, string(networkValue), unlocked.Wallet.ID, unlocked.Account.Index, args[0], args[1], memo)
			if err != nil {
				return renderError(err)
			}
			fmt.Println(proposal.ID)
			fmt.Fprintf(os.Stderr, "Proposal saved: %s/%s.json\n", store.ProposalDir(), proposal.ID)
			return nil
		},
	})
	cmd.Commands()[0].Flags().StringVar(&memo, "memo", "", "optional text memo")

	cmd.AddCommand(&cobra.Command{
		Use:   "sign <proposal-id>",
		Short: "Sign a stored multisig proposal with the active account",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			unlocked, err := unlockActiveWallet()
			if err != nil {
				return renderError(err)
			}
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			service := multisig.NewService(store)
			proposal, err := service.SignProposal(unlocked.Secret, args[0])
			if err != nil {
				return renderError(err)
			}
			fmt.Println(proposal.ID)
			fmt.Fprintf(os.Stderr, "Proposal signatures: %d\n", len(proposal.Signers))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "submit <proposal-id>",
		Short: "Submit a stored multisig proposal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			service := multisig.NewService(store)
			hash, err := service.SubmitProposal(args[0])
			if err != nil {
				return renderError(err)
			}
			fmt.Println(hash)
			return nil
		},
	})

	return cmd
}

func newWalletCmd() *cobra.Command {
	var createName string
	var createWords int
	var importName string
	var deriveName string
	var deriveIndex uint32
	var switchIndex uint32

	cmd := &cobra.Command{
		Use:   "wallet",
		Short: "Manage encrypted HD wallet roots and derived accounts",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "create",
		Short: "Create a new encrypted HD wallet root",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			passphrase, err := promptPassphrase("Choose wallet passphrase: ", true)
			if err != nil {
				return err
			}
			summary, mnemonic, err := store.CreateWallet(wallet.CreateOptions{
				Name:       createName,
				Passphrase: passphrase,
				Words:      createWords,
			})
			if err != nil {
				return renderError(err)
			}
			fmt.Println(summary.Accounts[0].Address)
			fmt.Fprintf(os.Stderr, "Created wallet %q with active account %d\n", summary.Name, summary.Accounts[0].Index)
			fmt.Fprintf(os.Stderr, "Mnemonic (store offline): %s\n", mnemonic)
			fmt.Fprintf(os.Stderr, "Encrypted wallet DB: %s\n", store.DBDir())
			return nil
		},
	})
	cmd.Commands()[0].Flags().StringVar(&createName, "name", "", "wallet name")
	cmd.Commands()[0].Flags().IntVar(&createWords, "words", 24, "mnemonic size: 12 or 24")

	cmd.AddCommand(&cobra.Command{
		Use:   "import <mnemonic>",
		Short: "Import an existing BIP39 mnemonic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			passphrase, err := promptPassphrase("Choose wallet passphrase: ", true)
			if err != nil {
				return err
			}
			summary, err := store.ImportWallet(wallet.ImportOptions{
				Name:       importName,
				Mnemonic:   args[0],
				Passphrase: passphrase,
			})
			if err != nil {
				return renderError(err)
			}
			fmt.Println(summary.Accounts[0].Address)
			fmt.Fprintf(os.Stderr, "Imported wallet %q with active account %d\n", summary.Name, summary.Accounts[0].Index)
			fmt.Fprintf(os.Stderr, "Encrypted wallet DB: %s\n", store.DBDir())
			return nil
		},
	})
	cmd.Commands()[1].Flags().StringVar(&importName, "name", "", "wallet name")

	cmd.AddCommand(&cobra.Command{
		Use:   "derive <wallet-id|name>",
		Short: "Derive and persist another account under a wallet root",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			passphrase, err := promptPassphrase("Wallet passphrase: ", false)
			if err != nil {
				return err
			}
			account, err := store.Derive(args[0], passphrase, deriveIndex, deriveName)
			if err != nil {
				return renderError(err)
			}
			fmt.Println(account.Address)
			fmt.Fprintf(os.Stderr, "Derived account %d at %s under wallet %s\n", account.Index, account.Path, account.WalletID)
			return nil
		},
	})
	cmd.Commands()[2].Flags().Uint32Var(&deriveIndex, "index", 1, "HD account index to derive")
	cmd.Commands()[2].Flags().StringVar(&deriveName, "name", "", "optional derived account label")

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List wallet roots and locally derived accounts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			items, err := store.ListWallets()
			if err != nil {
				return renderError(err)
			}
			for _, item := range items {
				for _, account := range item.Accounts {
					status := "inactive"
					if item.Active && account.Index == item.ActiveAccountIndex {
						status = "active"
					}
					fmt.Printf("%s\t%s\t%d\t%s\t%s\t%s\n", item.ID, item.Name, account.Index, account.Address, status, account.Path)
				}
			}
			fmt.Fprintf(os.Stderr, "Encrypted wallet DB: %s\n", store.DBDir())
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "switch <wallet-id|name>",
		Short: "Switch the active wallet root and account index",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			if err := store.SetActiveWallet(args[0], switchIndex); err != nil {
				return renderError(err)
			}
			summary, err := store.Wallet(args[0])
			if err != nil {
				return renderError(err)
			}
			for _, account := range summary.Accounts {
				if account.Index == switchIndex {
					fmt.Println(account.Address)
					fmt.Fprintf(os.Stderr, "Active wallet switched to %q account %d\n", summary.Name, switchIndex)
					return nil
				}
			}
			return renderError(wallet.ErrAccountNotDerived)
		},
	})
	cmd.Commands()[4].Flags().Uint32Var(&switchIndex, "account-index", 0, "derived account index to activate")

	cmd.AddCommand(&cobra.Command{
		Use:   "address",
		Short: "Print the active derived wallet address",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newStore()
			if err != nil {
				return err
			}
			defer store.Close()
			cfg, err := store.Config()
			if err != nil {
				return renderError(err)
			}
			summary, err := store.Wallet(cfg.ActiveWalletID)
			if err != nil {
				return renderError(err)
			}
			for _, account := range summary.Accounts {
				if account.Index == cfg.ActiveAccountIndex {
					fmt.Println(account.Address)
					return nil
				}
			}
			return renderError(wallet.ErrAccountNotDerived)
		},
	})

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
			info.Name = unlocked.Wallet.Name
			if !info.Funded {
				return fmt.Errorf("Account not funded. Run `nb fund` on testnet or send XLM to %s", info.Address)
			}
			fmt.Printf("%s\t%d\t%s\t%s\t%s\n", info.Name, unlocked.Account.Index, networkValue, nebula.AssetCodeXLM, nativeBalance(info))
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
			client, _, networkValue, err := resolveClient(state)
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
			fmt.Println(hash)
			fmt.Fprintf(os.Stderr, "Friendbot funding submitted on %s\n", networkValue)
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
			networkValue, err := resolveNetwork(state.networkOverride)
			if err != nil {
				return renderError(err)
			}
			fmt.Println(networkValue)
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
			defer store.Close()
			networkValue := nebula.Network(strings.ToLower(strings.TrimSpace(args[0])))
			if !networkValue.Valid() {
				return renderError(nebula.ErrUnsupportedNetwork)
			}
			if err := store.SetNetwork(string(networkValue)); err != nil {
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
			defer store.Close()
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

func newManCmd(root *cobra.Command) *cobra.Command {
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

func newStore() (*wallet.Store, error) {
	return wallet.NewStore()
}

func resolveClient(state *appState) (*nebula.Client, unlockedSession, nebula.Network, error) {
	unlocked, err := unlockActiveWallet()
	if err != nil {
		return nil, unlockedSession{}, "", err
	}
	networkValue, err := resolveNetwork(state.networkOverride)
	if err != nil {
		return nil, unlockedSession{}, "", err
	}
	client, err := nebula.NewClient(unlocked.Secret, networkValue)
	if err != nil {
		return nil, unlockedSession{}, "", err
	}
	if state.verbose {
		fmt.Fprintf(os.Stderr, "Using wallet %q account %d (%s) on %s\n", unlocked.Wallet.Name, unlocked.Account.Index, unlocked.Account.Address, networkValue)
	}
	return client, unlocked, networkValue, nil
}

func unlockActiveWallet() (unlockedSession, error) {
	store, err := newStore()
	if err != nil {
		return unlockedSession{}, err
	}
	defer store.Close()
	passphrase, err := promptPassphrase("Wallet passphrase: ", false)
	if err != nil {
		return unlockedSession{}, err
	}
	summary, account, secret, err := store.ActiveAccount(passphrase)
	if err != nil {
		return unlockedSession{}, err
	}
	return unlockedSession{Wallet: summary, Account: account, Secret: secret}, nil
}

func resolveNetwork(override string) (nebula.Network, error) {
	if strings.TrimSpace(override) != "" {
		networkValue := nebula.Network(strings.ToLower(strings.TrimSpace(override)))
		if !networkValue.Valid() {
			return "", nebula.ErrUnsupportedNetwork
		}
		return networkValue, nil
	}
	store, err := newStore()
	if err != nil {
		return "", err
	}
	defer store.Close()
	networkValue := nebula.Network(store.CurrentNetwork())
	if !networkValue.Valid() {
		return "", nebula.ErrUnsupportedNetwork
	}
	return networkValue, nil
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
	case errors.Is(err, wallet.ErrWalletNotFound):
		return fmt.Errorf("wallet not found")
	case errors.Is(err, wallet.ErrInvalidPassphrase):
		return fmt.Errorf("invalid passphrase")
	case errors.Is(err, wallet.ErrInvalidMnemonic):
		return fmt.Errorf("invalid mnemonic")
	case errors.Is(err, wallet.ErrInvalidWordsCount):
		return fmt.Errorf("mnemonic words must be 12 or 24")
	case errors.Is(err, wallet.ErrAccountNotDerived):
		return fmt.Errorf("account index not derived in active wallet")
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

func parseSince(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 24 * time.Hour, nil
	}
	return time.ParseDuration(raw)
}

func syncTargets(account string) ([]string, error) {
	if strings.TrimSpace(account) != "" {
		return []string{strings.TrimSpace(account)}, nil
	}
	store, err := newStore()
	if err != nil {
		return nil, err
	}
	defer store.Close()
	items, err := store.ListWallets()
	if err != nil {
		return nil, err
	}
	targets := []string{}
	for _, item := range items {
		for _, derived := range item.Accounts {
			targets = append(targets, derived.Address)
		}
	}
	return targets, nil
}
