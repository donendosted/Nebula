package main

import (
	"fmt"
	"os"

	"nebula/internal/wallet"

	"github.com/spf13/cobra"
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

	cmd.AddCommand(newWalletCmd(state))
	cmd.AddCommand(newBalanceCmd(state))
	cmd.AddCommand(newSendCmd(state))
	cmd.AddCommand(newHistoryCmd(state))
	cmd.AddCommand(newFundCmd(state))
	cmd.AddCommand(newNetworkCmd(state))

	return cmd
}

func newWalletCmd(state *appState) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wallet",
		Short: "Manage the local wallet",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "create",
		Short: "Create a new wallet",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(state)
			if err != nil {
				return err
			}

			result, err := svc.CreateWallet()
			if err != nil {
				return err
			}

			fmt.Println(result.Address)
			fmt.Fprintf(os.Stderr, "Created wallet and set active: %s\n", result.Address)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "import <secret>",
		Short: "Import an existing Stellar secret",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(state)
			if err != nil {
				return err
			}

			result, err := svc.ImportWallet(args[0])
			if err != nil {
				return err
			}

			fmt.Println(result.Address)
			fmt.Fprintf(os.Stderr, "Imported wallet and set active: %s\n", result.Address)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "address",
		Short: "Print the wallet address",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(state)
			if err != nil {
				return err
			}

			address, err := svc.Address()
			if err != nil {
				return err
			}

			fmt.Println(address)
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List saved wallets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(state)
			if err != nil {
				return err
			}

			wallets, err := svc.ListWallets()
			if err != nil {
				return err
			}

			activeAddress, err := svc.Address()
			if err != nil {
				return err
			}

			for _, walletData := range wallets {
				status := "inactive"
				if walletData.Address == activeAddress {
					status = "active"
				}
				fmt.Printf("%s\t%s\n", walletData.Address, status)
			}
			fmt.Fprintf(os.Stderr, "Listed %d wallet(s)\n", len(wallets))
			return nil
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "switch <address>",
		Short: "Switch the active wallet",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(state)
			if err != nil {
				return err
			}

			walletData, err := svc.SwitchWallet(args[0])
			if err != nil {
				return err
			}

			fmt.Println(walletData.Address)
			fmt.Fprintf(os.Stderr, "Active wallet switched to %s\n", walletData.Address)
			return nil
		},
	})

	return cmd
}

func newBalanceCmd(state *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "balance",
		Short: "Show the native XLM balance",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(state)
			if err != nil {
				return err
			}

			networkValue, err := svc.CurrentNetwork(state.networkOverride)
			if err != nil {
				return err
			}

			info, err := svc.AccountInfo(networkValue)
			if err != nil {
				return err
			}

			if !info.Funded {
				return fmt.Errorf("Account not funded. Run `nb fund` on testnet or send XLM to %s", info.Address)
			}

			fmt.Printf("%s\t%s\t%s\n", networkValue, wallet.AssetCodeXLM, wallet.NativeBalanceFromInfo(info))
			return nil
		},
	}
}

func newSendCmd(state *appState) *cobra.Command {
	var memo string

	cmd := &cobra.Command{
		Use:   "send <address> <amount>",
		Short: "Send XLM",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(state)
			if err != nil {
				return err
			}

			networkValue, err := svc.CurrentNetwork(state.networkOverride)
			if err != nil {
				return err
			}

			result, err := svc.SendXLM(networkValue, args[0], args[1], memo)
			if err != nil {
				if err == wallet.ErrAccountNotFunded {
					return fmt.Errorf("Account not funded. Run `nb fund` first")
				}
				return err
			}

			fmt.Printf("%s\t%s\t%s\n", result.Amount, result.AssetCode, result.Hash)
			if memo == "" {
				fmt.Fprintln(os.Stderr, "Suggestion: use --memo to add note")
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
			svc, err := newService(state)
			if err != nil {
				return err
			}

			networkValue, err := svc.CurrentNetwork(state.networkOverride)
			if err != nil {
				return err
			}

			history, err := svc.History(networkValue, limit)
			if err != nil {
				if err == wallet.ErrAccountNotFunded {
					return fmt.Errorf("Account not funded. Run `nb fund` first")
				}
				return err
			}

			for _, entry := range history {
				fmt.Printf(
					"%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
					entry.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
					entry.Hash,
					entry.Type,
					entry.Direction,
					entry.Amount,
					entry.AssetCode,
					entry.Counterparty,
				)
			}
			fmt.Fprintf(os.Stderr, "History loaded: %d transaction(s) on %s\n", len(history), networkValue)
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", wallet.DefaultHistoryLimit, "number of entries to fetch")
	return cmd
}

func newFundCmd(state *appState) *cobra.Command {
	return &cobra.Command{
		Use:   "fund",
		Short: "Fund the account with Friendbot on testnet",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := newService(state)
			if err != nil {
				return err
			}

			networkValue, err := svc.CurrentNetwork(state.networkOverride)
			if err != nil {
				return err
			}

			hash, err := svc.Fund(networkValue)
			if err != nil {
				return err
			}

			fmt.Println(hash)
			fmt.Fprintf(os.Stderr, "Funded wallet on %s: %s\n", networkValue, hash)
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
			svc, err := newService(state)
			if err != nil {
				return err
			}

			current, err := svc.CurrentNetwork("")
			if err != nil {
				return err
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
			svc, err := newService(state)
			if err != nil {
				return err
			}

			networkValue := wallet.Network(args[0])
			if !networkValue.Valid() {
				return wallet.ErrUnsupportedNetwork
			}

			if err := svc.SetNetwork(networkValue); err != nil {
				return err
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
			svc, err := newService(state)
			if err != nil {
				return err
			}

			networkValue, err := svc.ToggleNetwork()
			if err != nil {
				return err
			}

			fmt.Println(networkValue)
			return nil
		},
	})

	return cmd
}

func newService(state *appState) (*wallet.Service, error) {
	return wallet.NewService(wallet.ServiceOptions{Verbose: state.verbose})
}
