package wallet

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	configDirName   = "nebula"
	secretFileName  = "secret"
	configFileName  = "config.json"
	walletsDirName  = "wallets"
	defaultFileMode = 0o600
)

type configFile struct {
	Network      Network `json:"network"`
	ActiveWallet string  `json:"active_wallet"`
}

type Storage struct {
	baseDir string
}

func NewStorage() (*Storage, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("resolve config dir: %w", err)
	}
	return &Storage{baseDir: filepath.Join(configRoot, configDirName)}, nil
}

func (s *Storage) BaseDir() string {
	return s.baseDir
}

func (s *Storage) SecretPath() string {
	return filepath.Join(s.baseDir, secretFileName)
}

func (s *Storage) WalletsDir() string {
	return filepath.Join(s.baseDir, walletsDirName)
}

func (s *Storage) WalletPath(address string) string {
	return filepath.Join(s.WalletsDir(), address+".secret")
}

func (s *Storage) ConfigPath() string {
	return filepath.Join(s.baseDir, configFileName)
}

func (s *Storage) ActiveWalletPath() (string, error) {
	walletData, err := s.LoadWallet()
	if err != nil {
		return "", err
	}
	return walletData.SecretPath, nil
}

func (s *Storage) Ensure() error {
	if err := os.MkdirAll(s.baseDir, 0o700); err != nil {
		return err
	}
	return os.MkdirAll(s.WalletsDir(), 0o700)
}

func (s *Storage) LoadWallet() (Wallet, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return Wallet{}, err
		}
		cfg = configFile{}
	}

	if cfg.ActiveWallet != "" {
		return s.loadWalletByAddress(cfg.ActiveWallet)
	}

	legacyWallet, err := s.loadLegacyWallet()
	if err == nil {
		return legacyWallet, nil
	}
	if errors.Is(err, ErrWalletNotFound) {
		return Wallet{}, ErrWalletNotFound
	}
	return Wallet{}, err
}

func (s *Storage) SaveWallet(secret string) (Wallet, error) {
	full, err := ParseSecret(secret)
	if err != nil {
		return Wallet{}, err
	}

	walletData := Wallet{
		Address:    full.Address(),
		Secret:     full.Seed(),
		SecretPath: s.WalletPath(full.Address()),
		Active:     true,
	}

	if err := s.Ensure(); err != nil {
		return Wallet{}, fmt.Errorf("prepare config dir: %w", err)
	}

	if err := os.WriteFile(s.WalletPath(walletData.Address), []byte(walletData.Secret), defaultFileMode); err != nil {
		return Wallet{}, fmt.Errorf("write wallet: %w", err)
	}

	cfg, err := s.loadConfig()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		if !errors.Is(err, ErrCorruptWallet) && !errors.Is(err, ErrWalletNotFound) {
			return Wallet{}, err
		}
		cfg = configFile{}
	}
	cfg.ActiveWallet = walletData.Address

	if err := s.saveConfig(cfg); err != nil {
		return Wallet{}, err
	}

	return walletData, nil
}

func (s *Storage) LoadNetwork() (Network, error) {
	cfg, err := s.loadConfig()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || errors.Is(err, ErrWalletNotFound) {
			return NetworkTestnet, nil
		}
		return "", err
	}
	if cfg.Network == "" {
		return NetworkTestnet, nil
	}

	if !cfg.Network.Valid() {
		return "", ErrCorruptWallet
	}

	return cfg.Network, nil
}

func (s *Storage) SaveNetwork(network Network) error {
	if !network.Valid() {
		return ErrUnsupportedNetwork
	}

	cfg, err := s.loadConfig()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, ErrWalletNotFound) && !errors.Is(err, ErrCorruptWallet) {
			return err
		}
		cfg = configFile{}
	}

	cfg.Network = network
	return s.saveConfig(cfg)
}

func (s *Storage) ListWallets() ([]Wallet, error) {
	if err := s.Ensure(); err != nil {
		return nil, fmt.Errorf("prepare config dir: %w", err)
	}

	entries, err := os.ReadDir(s.WalletsDir())
	if err != nil {
		return nil, fmt.Errorf("read wallets dir: %w", err)
	}

	wallets := make([]Wallet, 0, len(entries))
	activeAddress := ""
	cfg, err := s.loadConfig()
	if err == nil {
		activeAddress = cfg.ActiveWallet
	} else if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, ErrCorruptWallet) {
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".secret") {
			continue
		}

		address := strings.TrimSuffix(entry.Name(), ".secret")
		walletData, err := s.loadWalletByAddress(address)
		if err != nil {
			return nil, err
		}
		walletData.Active = walletData.Address == activeAddress
		wallets = append(wallets, walletData)
	}

	if len(wallets) == 0 {
		legacyWallet, err := s.loadLegacyWallet()
		if err == nil {
			legacyWallet.Active = true
			return []Wallet{legacyWallet}, nil
		}
		if errors.Is(err, ErrWalletNotFound) {
			return nil, ErrWalletNotFound
		}
		return nil, err
	}

	sort.Slice(wallets, func(i, j int) bool {
		if wallets[i].Active != wallets[j].Active {
			return wallets[i].Active
		}
		return wallets[i].Address < wallets[j].Address
	})

	return wallets, nil
}

func (s *Storage) SwitchWallet(address string) (Wallet, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return Wallet{}, ErrWalletNotFound
	}

	walletData, err := s.loadWalletByAddress(address)
	if err != nil {
		return Wallet{}, err
	}

	cfg, err := s.loadConfig()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, ErrWalletNotFound) && !errors.Is(err, ErrCorruptWallet) {
			return Wallet{}, err
		}
		cfg = configFile{}
	}

	cfg.ActiveWallet = walletData.Address
	if err := s.saveConfig(cfg); err != nil {
		return Wallet{}, err
	}

	return walletData, nil
}

func (s *Storage) loadWalletByAddress(address string) (Wallet, error) {
	secretBytes, err := os.ReadFile(s.WalletPath(address))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Wallet{}, ErrWalletNotFound
		}
		return Wallet{}, fmt.Errorf("read wallet: %w", err)
	}

	secret := strings.TrimSpace(string(secretBytes))
	if secret == "" {
		return Wallet{}, ErrCorruptWallet
	}

	full, err := ParseSecret(secret)
	if err != nil {
		return Wallet{}, ErrCorruptWallet
	}

	return Wallet{
		Address:    full.Address(),
		Secret:     full.Seed(),
		SecretPath: s.WalletPath(full.Address()),
	}, nil
}

func (s *Storage) loadLegacyWallet() (Wallet, error) {
	secretBytes, err := os.ReadFile(s.SecretPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Wallet{}, ErrWalletNotFound
		}
		return Wallet{}, fmt.Errorf("read wallet: %w", err)
	}

	secret := strings.TrimSpace(string(secretBytes))
	if secret == "" {
		return Wallet{}, ErrCorruptWallet
	}

	full, err := ParseSecret(secret)
	if err != nil {
		return Wallet{}, ErrCorruptWallet
	}

	return Wallet{
		Address:    full.Address(),
		Secret:     full.Seed(),
		SecretPath: s.SecretPath(),
		Active:     true,
	}, nil
}

func (s *Storage) loadConfig() (configFile, error) {
	raw, err := os.ReadFile(s.ConfigPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return configFile{}, os.ErrNotExist
		}
		return configFile{}, fmt.Errorf("read config: %w", err)
	}

	var cfg configFile
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return configFile{}, ErrCorruptWallet
	}

	return cfg, nil
}

func (s *Storage) saveConfig(cfg configFile) error {
	if err := s.Ensure(); err != nil {
		return fmt.Errorf("prepare config dir: %w", err)
	}

	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if err := os.WriteFile(s.ConfigPath(), append(payload, '\n'), defaultFileMode); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}
