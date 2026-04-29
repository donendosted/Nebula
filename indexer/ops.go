package indexer

import (
	"fmt"

	"github.com/stellar/go-stellar-sdk/protocols/horizon/operations"
)

func operationRecord(networkName, account string, record operations.Operation) (Record, bool) {
	switch op := record.(type) {
	case operations.Payment:
		if op.Asset.Type != "native" {
			return Record{}, false
		}
		direction := "out"
		counterparty := op.To
		if op.To == account {
			direction = "in"
			counterparty = op.From
		}
		return Record{
			Account:      account,
			Hash:         op.TransactionHash,
			Timestamp:    op.LedgerCloseTime,
			Amount:       op.Amount,
			Direction:    direction,
			Counterparty: counterparty,
			Type:         op.Base.Type,
			AssetCode:    "XLM",
			ExplorerURL:  explorerURL(networkName, op.TransactionHash),
			Successful:   op.TransactionSuccessful,
		}, true
	case operations.CreateAccount:
		direction := "out"
		counterparty := op.Account
		if op.Account == account {
			direction = "in"
			counterparty = op.Funder
		}
		return Record{
			Account:      account,
			Hash:         op.TransactionHash,
			Timestamp:    op.LedgerCloseTime,
			Amount:       op.StartingBalance,
			Direction:    direction,
			Counterparty: counterparty,
			Type:         op.Type,
			AssetCode:    "XLM",
			ExplorerURL:  explorerURL(networkName, op.TransactionHash),
			Successful:   op.TransactionSuccessful,
		}, true
	default:
		return Record{}, false
	}
}

func explorerURL(networkName, hash string) string {
	explorerNetwork := "testnet"
	if networkName == "mainnet" {
		explorerNetwork = "public"
	}
	return fmt.Sprintf("https://stellar.expert/explorer/%s/tx/%s", explorerNetwork, hash)
}
