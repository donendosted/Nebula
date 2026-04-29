package indexer

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

func TestStatsAndSearch(t *testing.T) {
	store, err := NewStoreAt(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })

	now := time.Now().UTC()
	err = store.db.Update(func(txn *badger.Txn) error {
		records := []Record{
			{Account: "G1", Hash: "h1", Timestamp: now, Amount: "2.5", Direction: "out", LatencyMS: 100},
			{Account: "G1", Hash: "h2", Timestamp: now.Add(-time.Hour), Amount: "5", Direction: "in", LatencyMS: 300},
		}
		for _, record := range records {
			payload, err := json.Marshal(record)
			if err != nil {
				return err
			}
			if err := txn.Set(recordKey(record.Account, record.Hash), payload); err != nil {
				return err
			}
			if err := txn.Set(accountIndexKey(record.Account, record.Timestamp, record.Hash), []byte(record.Hash)); err != nil {
				return err
			}
			if err := txn.Set(timeIndexKey(record.Timestamp, record.Account, record.Hash), []byte(record.Hash)); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)

	records, err := store.SearchAccount("G1", 2*time.Hour)
	require.NoError(t, err)
	require.Len(t, records, 2)

	stats, err := store.Stats("G1")
	require.NoError(t, err)
	require.Equal(t, 2, stats.TotalTransactions)
	require.Equal(t, 2.5, stats.TotalVolumeSent)
	require.Equal(t, 5.0, stats.TotalVolumeRecv)
	require.Equal(t, 200.0, stats.AverageLatencyMS)
}
