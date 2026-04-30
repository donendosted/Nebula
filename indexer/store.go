package indexer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"nebula/internal/metrics"
	"nebula/stellar"
	"nebula/wallet"

	"github.com/dgraph-io/badger/v4"
)

// Store manages a local transaction cache and analytics indexes.
type Store struct {
	rootDir string
	dbDir   string
	db      *badger.DB
}

// NewStore opens the default index database at ~/.nebula/index.db.
func NewStore() (*Store, error) {
	walletStore, err := wallet.NewStore()
	if err != nil {
		return nil, err
	}
	defer walletStore.Close()
	return NewStoreAt(walletStore.IndexDir())
}

// NewStoreAt opens an index store at the provided db directory.
func NewStoreAt(dbDir string) (*Store, error) {
	rootDir := filepath.Dir(dbDir)
	if err := os.MkdirAll(rootDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dbDir, 0o700); err != nil {
		return nil, err
	}
	opts := badger.DefaultOptions(dbDir)
	opts.Logger = nil
	opts.ValueDir = dbDir
	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open index db: %w", err)
	}
	return &Store{rootDir: rootDir, dbDir: dbDir, db: db}, nil
}

// Close releases database resources.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DBDir returns the underlying BadgerDB directory.
func (s *Store) DBDir() string {
	return s.dbDir
}

// SyncAccount refreshes up to limit recent native payment records for one account.
func (s *Store) SyncAccount(client *stellar.Client, account string, limit int) (int, error) {
	if limit <= 0 {
		limit = maxSyncLimit
	}
	if limit > maxSyncLimit {
		limit = maxSyncLimit
	}
	page, err := client.Payments(account, limit)
	if err != nil {
		return 0, fmt.Errorf("load payments: %w", err)
	}
	records := make([]Record, 0, len(page.Embedded.Records))
	for _, op := range page.Embedded.Records {
		record, ok := operationRecord(client.Network(), account, op)
		if ok {
			records = append(records, record)
		}
	}
	if err := s.db.Update(func(txn *badger.Txn) error {
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
		return txn.Set(metaKey("last_sync:"+account), []byte(time.Now().UTC().Format(time.RFC3339)))
	}); err != nil {
		return 0, err
	}
	metrics.RecordWalletAction("sync", strings.TrimSpace(account))
	total, err := s.CountRecords()
	if err == nil {
		metrics.SetIndexedTxTotal(total)
	}
	return len(records), nil
}

// SearchAccount returns locally indexed records for an account, optionally filtered by recency.
func (s *Store) SearchAccount(account string, since time.Duration) ([]Record, error) {
	if since <= 0 {
		since = defaultDuration
	}
	cutoff := time.Now().UTC().Add(-since)
	results := []Record{}
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		prefix := []byte(acctKeyPrefix + strings.TrimSpace(account) + ":")
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := string(item.Key())
			parts := strings.Split(key, ":")
			if len(parts) < 4 {
				continue
			}
			unixNanos, err := strconv.ParseInt(parts[2], 10, 64)
			if err != nil {
				continue
			}
			ts := time.Unix(0, unixNanos).UTC()
			if ts.Before(cutoff) {
				continue
			}
			hash := parts[3]
			record, err := s.record(txn, account, hash)
			if err != nil {
				return err
			}
			results = append(results, record)
		}
		return nil
	})
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})
	return results, err
}

// SearchSince returns locally indexed records newer than the given duration across all accounts.
func (s *Store) SearchSince(since time.Duration) ([]Record, error) {
	if since <= 0 {
		since = defaultDuration
	}
	cutoff := time.Now().UTC().Add(-since)
	seen := map[string]bool{}
	results := []Record{}
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek([]byte(timeKeyPrefix)); it.ValidForPrefix([]byte(timeKeyPrefix)); it.Next() {
			parts := strings.Split(string(it.Item().Key()), ":")
			if len(parts) < 4 {
				continue
			}
			unixNanos, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				continue
			}
			ts := time.Unix(0, unixNanos).UTC()
			if ts.Before(cutoff) {
				continue
			}
			account := parts[2]
			hash := parts[3]
			if seen[account+":"+hash] {
				continue
			}
			record, err := s.record(txn, account, hash)
			if err != nil {
				return err
			}
			seen[account+":"+hash] = true
			results = append(results, record)
		}
		return nil
	})
	sort.Slice(results, func(i, j int) bool {
		return results[i].Timestamp.After(results[j].Timestamp)
	})
	return results, err
}

// Stats computes aggregate metrics over local records for one account or all accounts.
func (s *Store) Stats(account string) (Stats, error) {
	var records []Record
	var err error
	if strings.TrimSpace(account) != "" {
		records, err = s.SearchAccount(account, 3650*24*time.Hour)
	} else {
		records, err = s.SearchSince(3650 * 24 * time.Hour)
	}
	if err != nil {
		return Stats{}, err
	}
	stats := Stats{TotalTransactions: len(records)}
	latencyCount := 0
	for _, record := range records {
		amount, _ := strconv.ParseFloat(record.Amount, 64)
		switch record.Direction {
		case "out":
			stats.TotalVolumeSent += amount
		case "in":
			stats.TotalVolumeRecv += amount
		}
		if record.LatencyMS > 0 {
			stats.AverageLatencyMS += float64(record.LatencyMS)
			latencyCount++
		}
	}
	if latencyCount > 0 {
		stats.AverageLatencyMS /= float64(latencyCount)
	}
	return stats, nil
}

// CountRecords returns the number of indexed transaction records across all accounts.
func (s *Store) CountRecords() (int, error) {
	total := 0
	err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek([]byte(txKeyPrefix)); it.ValidForPrefix([]byte(txKeyPrefix)); it.Next() {
			total++
		}
		return nil
	})
	return total, err
}

func (s *Store) record(txn *badger.Txn, account, hash string) (Record, error) {
	item, err := txn.Get(recordKey(account, hash))
	if err != nil {
		return Record{}, err
	}
	var record Record
	err = item.Value(func(val []byte) error { return json.Unmarshal(val, &record) })
	return record, err
}

func recordKey(account, hash string) []byte {
	return []byte(txKeyPrefix + strings.TrimSpace(account) + ":" + strings.TrimSpace(hash))
}

func accountIndexKey(account string, ts time.Time, hash string) []byte {
	return []byte(acctKeyPrefix + strings.TrimSpace(account) + ":" + fmt.Sprintf("%020d", ts.UTC().UnixNano()) + ":" + strings.TrimSpace(hash))
}

func timeIndexKey(ts time.Time, account, hash string) []byte {
	return []byte(timeKeyPrefix + fmt.Sprintf("%020d", ts.UTC().UnixNano()) + ":" + strings.TrimSpace(account) + ":" + strings.TrimSpace(hash))
}

func metaKey(key string) []byte {
	return []byte(metaKeyPrefix + key)
}
