package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"nebula/internal/wallet"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type viewMode string

const (
	modeHome     viewMode = "home"
	modeSend     viewMode = "send"
	modeHistory  viewMode = "history"
	modeSettings viewMode = "settings"
	modeWallets  viewMode = "wallets"
)

type walletLoadedMsg struct {
	info wallet.AccountInfo
	err  error
}

type historyLoadedMsg struct {
	items []wallet.HistoryEntry
	err   error
}

type sendResultMsg struct {
	result wallet.SendResult
	err    error
}

type fundedMsg struct {
	hash string
	err  error
}

type networkChangedMsg struct {
	network wallet.Network
	err     error
}

type walletListLoadedMsg struct {
	items []wallet.Wallet
	err   error
}

type walletSwitchedMsg struct {
	wallet wallet.Wallet
	err    error
}

type model struct {
	service *wallet.Service
	mode    viewMode

	info       wallet.AccountInfo
	history    []wallet.HistoryEntry
	wallets    []wallet.Wallet
	network    wallet.Network
	loading    bool
	status     string
	errMessage string
	walletNote string

	addressInput textinput.Model
	amountInput  textinput.Model
	memoInput    textinput.Model
	focusIndex   int
	walletCursor int

	width  int
	height int
}

func main() {
	svc, err := wallet.NewService(wallet.ServiceOptions{})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	initialNetwork, err := svc.CurrentNetwork("")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	m := newModel(svc, initialNetwork)
	program := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newModel(service *wallet.Service, networkValue wallet.Network) model {
	addressInput := textinput.New()
	addressInput.Placeholder = "Destination address"
	addressInput.CharLimit = 128
	addressInput.Width = 56

	amountInput := textinput.New()
	amountInput.Placeholder = "Amount"
	amountInput.CharLimit = 32
	amountInput.Width = 20

	memoInput := textinput.New()
	memoInput.Placeholder = "Memo (optional)"
	memoInput.CharLimit = 28
	memoInput.Width = 28

	addressInput.Focus()

	return model{
		service:      service,
		mode:         modeHome,
		network:      networkValue,
		loading:      true,
		addressInput: addressInput,
		amountInput:  amountInput,
		memoInput:    memoInput,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.refreshAccountCmd(), m.loadWalletsCmd())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case walletLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.info = msg.info
		m.status = fmt.Sprintf("Refreshed %s", time.Now().Format("15:04:05"))
		m.errMessage = ""
		return m, nil
	case historyLoadedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.history = msg.items
		m.status = fmt.Sprintf("Loaded %d entries", len(msg.items))
		m.errMessage = ""
		return m, nil
	case sendResultMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.status = fmt.Sprintf("Sent %s %s: %s", msg.result.Amount, msg.result.AssetCode, msg.result.Hash)
		m.errMessage = ""
		m.mode = modeHome
		m.addressInput.SetValue("")
		m.amountInput.SetValue("")
		m.memoInput.SetValue("")
		return m, m.refreshAccountCmd()
	case fundedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.status = fmt.Sprintf("Friendbot funded account: %s", msg.hash)
		m.errMessage = ""
		return m, m.refreshAccountCmd()
	case networkChangedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.network = msg.network
		m.status = fmt.Sprintf("Network switched to %s", msg.network)
		m.errMessage = ""
		return m, m.refreshAccountCmd()
	case walletListLoadedMsg:
		if msg.err != nil {
			m.loading = false
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.wallets = msg.items
		if m.walletCursor >= len(m.wallets) {
			m.walletCursor = max(0, len(m.wallets)-1)
		}
		if m.mode == modeWallets {
			m.status = fmt.Sprintf("Loaded %d wallet(s)", len(m.wallets))
		}
		m.errMessage = ""
		return m, nil
	case walletSwitchedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.status = fmt.Sprintf("Switched active wallet to %s", shortAddress(msg.wallet.Address))
		m.walletNote = msg.wallet.SecretPath
		m.errMessage = ""
		m.mode = modeHome
		return m, tea.Batch(m.refreshAccountCmd(), m.loadWalletsCmd())
	case tea.KeyMsg:
		switch m.mode {
		case modeSend:
			return m.updateSend(msg)
		case modeWallets:
			return m.updateWallets(msg)
		default:
			return m.updateGlobalKeys(msg)
		}
	}

	return m, nil
}

func (m model) updateGlobalKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		m.loading = true
		return m, m.refreshAccountCmd()
	case "n":
		m.loading = true
		return m, m.toggleNetworkCmd()
	case "s":
		m.mode = modeSend
		m.focusIndex = 0
		m.addressInput.Focus()
		m.amountInput.Blur()
		m.memoInput.Blur()
		return m, nil
	case "h":
		m.mode = modeHistory
		m.loading = true
		return m, m.loadHistoryCmd()
	case "c":
		m.mode = modeSettings
		return m, nil
	case "w":
		m.mode = modeWallets
		m.loading = true
		return m, m.loadWalletsCmd()
	case "f":
		if m.network == wallet.NetworkTestnet {
			m.loading = true
			return m, m.fundCmd()
		}
	case "esc":
		m.mode = modeHome
		return m, nil
	}

	return m, nil
}

func (m model) updateSend(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeHome
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.blurInputs()
		if msg.String() == "shift+tab" || msg.String() == "up" {
			m.focusIndex--
		} else {
			m.focusIndex++
		}
		if m.focusIndex < 0 {
			m.focusIndex = 2
		}
		if m.focusIndex > 2 {
			m.focusIndex = 0
		}
		m.focusCurrentInput()
		return m, nil
	case "enter":
		m.loading = true
		m.errMessage = ""
		return m, m.sendCmd()
	}

	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.addressInput, cmd = m.addressInput.Update(msg)
	case 1:
		m.amountInput, cmd = m.amountInput.Update(msg)
	case 2:
		m.memoInput, cmd = m.memoInput.Update(msg)
	}
	return m, cmd
}

func (m model) updateWallets(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeHome
		return m, nil
	case "up", "k":
		if len(m.wallets) == 0 {
			return m, nil
		}
		m.walletCursor--
		if m.walletCursor < 0 {
			m.walletCursor = len(m.wallets) - 1
		}
		return m, nil
	case "down", "j":
		if len(m.wallets) == 0 {
			return m, nil
		}
		m.walletCursor++
		if m.walletCursor >= len(m.wallets) {
			m.walletCursor = 0
		}
		return m, nil
	case "enter":
		selected, ok := m.selectedWallet()
		if !ok {
			return m, nil
		}
		m.loading = true
		m.errMessage = ""
		return m, m.switchWalletCmd(selected.Address)
	}
	return m, nil
}

func (m model) View() string {
	header := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("Nebula")
	infoStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).Width(max(48, m.width/2-2))
	sideStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1).Width(max(36, m.width/2-4))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))

	balanceText := "Account not funded"
	if m.info.Funded {
		balanceText = fmt.Sprintf("%s %s", wallet.NativeBalanceFromInfo(m.info), wallet.AssetCodeXLM)
	}

	left := infoStyle.Render(strings.Join([]string{
		"Wallet",
		"",
		fmt.Sprintf("Address: %s", emptyFallback(m.info.Address, "No wallet loaded")),
		fmt.Sprintf("Balance: %s", balanceText),
		fmt.Sprintf("Network: %s", m.network),
		fmt.Sprintf("Reserve: %s %s", m.info.Reserve, wallet.AssetCodeXLM),
	}, "\n"))

	right := sideStyle.Render(strings.Join([]string{
		"Actions",
		"",
		"[s] Send",
		"[h] History",
		"[r] Refresh",
		"[w] Wallets",
		"[n] Toggle network",
		"[c] Settings",
		"[f] Friendbot (testnet)",
		"[q] Quit",
	}, "\n"))

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)
	footer := okStyle.Render(m.status)
	if m.errMessage != "" {
		footer = errorStyle.Render(m.errMessage)
	}
	if m.loading {
		footer = "Loading..."
	}

	content := []string{header, "", body}
	switch m.mode {
	case modeSend:
		content = append(content, "", m.sendView())
	case modeHistory:
		content = append(content, "", m.historyView())
	case modeSettings:
		content = append(content, "", m.settingsView())
	case modeWallets:
		content = append(content, "", m.walletsView())
	}
	if m.walletNote != "" {
		content = append(content, "", "Wallet file: "+m.walletNote)
	}
	content = append(content, "", footer)
	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(content, "\n"))
}

func (m model) sendView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	return box.Render(strings.Join([]string{
		"Send",
		"",
		m.addressInput.View(),
		m.amountInput.View(),
		m.memoInput.View(),
		"",
		"Enter submits, Tab moves fields, Esc returns home.",
	}, "\n"))
}

func (m model) historyView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	if len(m.history) == 0 {
		return box.Render("History\n\nNo recent transactions.")
	}

	lines := []string{"History", ""}
	for _, item := range m.history {
		lines = append(lines, fmt.Sprintf(
			"%s  %s  %s %s %s  %s",
			item.CreatedAt.Format("2006-01-02 15:04"),
			shortHash(item.Hash),
			item.Direction,
			item.Amount,
			item.AssetCode,
			shortAddress(item.Counterparty),
		))
	}

	return box.Render(strings.Join(lines, "\n"))
}

func (m model) settingsView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	lines := []string{
		"Settings",
		"",
		fmt.Sprintf("Storage: %s", m.service.StorageDir()),
		fmt.Sprintf("Wallets: %s", m.service.WalletsDir()),
		fmt.Sprintf("Config: %s", m.service.ConfigPath()),
		fmt.Sprintf("Network: %s", m.network),
		"Press [w] to manage saved wallets.",
		"Press [n] to toggle and persist the selected network.",
		"Press [esc] to return home.",
	}
	if activePath, err := m.service.ActiveWalletPath(); err == nil {
		lines = append(lines, fmt.Sprintf("Active secret: %s", activePath))
	}
	return box.Render(strings.Join(lines, "\n"))
}

func (m model) walletsView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	if len(m.wallets) == 0 {
		return box.Render("Wallets\n\nNo saved wallets found.")
	}

	lines := []string{
		"Wallets",
		"",
		fmt.Sprintf("Saved in: %s", m.service.WalletsDir()),
		"",
	}
	for i, item := range m.wallets {
		cursor := " "
		if i == m.walletCursor {
			cursor = ">"
		}
		status := "saved"
		if item.Active {
			status = "active"
		}
		lines = append(lines, fmt.Sprintf("%s %s  %s", cursor, shortAddress(item.Address), status))
		lines = append(lines, "  "+item.SecretPath)
	}
	lines = append(lines, "", "Up/Down selects, Enter switches, Esc returns home.")
	return box.Render(strings.Join(lines, "\n"))
}

func (m model) refreshAccountCmd() tea.Cmd {
	service := m.service
	networkValue := m.network
	return func() tea.Msg {
		info, err := service.AccountInfo(networkValue)
		return walletLoadedMsg{info: info, err: normalizeUIError(err)}
	}
}

func (m model) loadHistoryCmd() tea.Cmd {
	service := m.service
	networkValue := m.network
	return func() tea.Msg {
		items, err := service.History(networkValue, wallet.DefaultHistoryLimit)
		return historyLoadedMsg{items: items, err: normalizeUIError(err)}
	}
}

func (m model) loadWalletsCmd() tea.Cmd {
	service := m.service
	return func() tea.Msg {
		items, err := service.ListWallets()
		return walletListLoadedMsg{items: items, err: normalizeUIError(err)}
	}
}

func (m model) sendCmd() tea.Cmd {
	service := m.service
	networkValue := m.network
	address := m.addressInput.Value()
	amount := m.amountInput.Value()
	memo := m.memoInput.Value()
	return func() tea.Msg {
		result, err := service.SendXLM(networkValue, address, amount, memo)
		return sendResultMsg{result: result, err: normalizeUIError(err)}
	}
}

func (m model) toggleNetworkCmd() tea.Cmd {
	service := m.service
	return func() tea.Msg {
		networkValue, err := service.ToggleNetwork()
		return networkChangedMsg{network: networkValue, err: normalizeUIError(err)}
	}
}

func (m model) fundCmd() tea.Cmd {
	service := m.service
	networkValue := m.network
	return func() tea.Msg {
		hash, err := service.Fund(networkValue)
		return fundedMsg{hash: hash, err: normalizeUIError(err)}
	}
}

func (m model) switchWalletCmd(address string) tea.Cmd {
	service := m.service
	return func() tea.Msg {
		walletData, err := service.SwitchWallet(address)
		return walletSwitchedMsg{wallet: walletData, err: normalizeUIError(err)}
	}
}

func (m *model) blurInputs() {
	m.addressInput.Blur()
	m.amountInput.Blur()
	m.memoInput.Blur()
}

func (m *model) focusCurrentInput() {
	switch m.focusIndex {
	case 0:
		m.addressInput.Focus()
	case 1:
		m.amountInput.Focus()
	case 2:
		m.memoInput.Focus()
	}
}

func normalizeUIError(err error) error {
	if err == nil {
		return nil
	}
	switch err {
	case wallet.ErrWalletNotFound:
		return fmt.Errorf("wallet not found. Use `nb wallet create` or `nb wallet import`")
	case wallet.ErrAccountNotFunded:
		return fmt.Errorf("Account not funded. Use `nb fund` on testnet")
	default:
		return err
	}
}

func (m model) selectedWallet() (wallet.Wallet, bool) {
	if len(m.wallets) == 0 || m.walletCursor < 0 || m.walletCursor >= len(m.wallets) {
		return wallet.Wallet{}, false
	}
	return m.wallets[m.walletCursor], true
}

func shortHash(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:6] + "..." + value[len(value)-6:]
}

func shortAddress(value string) string {
	if len(value) <= 14 {
		return value
	}
	return value[:7] + "..." + value[len(value)-7:]
}

func emptyFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
