package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nebula/indexer"
	"nebula/internal/db"
	"nebula/internal/metrics"
	"nebula/internal/monitoring"
	tuiview "nebula/internal/tui"
	"nebula/multisig"
	"nebula/stellar"
	"nebula/wallet"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type viewMode string

const (
	modeWelcome  viewMode = "welcome"
	modeLogin    viewMode = "login"
	modeHome     viewMode = "home"
	modeSend     viewMode = "send"
	modePropose  viewMode = "propose"
	modeHistory  viewMode = "history"
	modeMonitor  viewMode = "monitor"
	modeSettings viewMode = "settings"
	modeWallets  viewMode = "wallets"
	modeCreate   viewMode = "create"
	modeImport   viewMode = "import"
)

type walletItem struct {
	Wallet       wallet.WalletSummary
	Account      wallet.DerivedAccount
	Active       bool
	DisplayLabel string
}

type session struct {
	Wallet     wallet.WalletSummary
	Account    wallet.DerivedAccount
	Secret     string
	Passphrase string
}

type accountState struct {
	Address string
	Balance string
	Reserve string
	Funded  bool
}

type walletInventoryMsg struct {
	Items  []walletItem
	Active *walletItem
	Err    error
}

type loginMsg struct {
	Session session
	Err     error
}

type accountMsg struct {
	Account accountState
	Err     error
}

type historyMsg struct {
	Items []indexer.Record
	Err   error
}

type sendMsg struct {
	Hash string
	Err  error
}

type fundMsg struct {
	Hash string
	Err  error
}

type proposalMsg struct {
	Proposal multisig.Proposal
	Err      error
}

type monitorMsg struct {
	Snapshot metrics.Snapshot
}

type networkMsg struct {
	Network string
	Err     error
}

type walletSavedMsg struct {
	Session session
	Err     error
}

type clipboardMsg struct {
	Err error
}

type dashboardMsg struct {
	Err error
}

type model struct {
	store          *wallet.Store
	indexStore     *indexer.Store
	walletReadOnly bool
	network        string
	mode           viewMode

	activeItem *walletItem
	session    *session
	wallets    []walletItem
	history    []indexer.Record
	account    accountState
	monitoring metrics.Snapshot

	loading    bool
	status     string
	errMessage string
	quitArmed  bool

	loginInput  textinput.Model
	nameInput   textinput.Model
	secretInput textinput.Model
	passInput   textinput.Model
	sendToInput textinput.Model
	amountInput textinput.Model
	memoInput   textinput.Model
	focusIndex  int

	walletCursor int
	width        int
	height       int
}

func main() {
	metrics.EnsureServer()
	handles, err := db.OpenForTUI()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer handles.Close()
	m := newModel(handles)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newModel(handles *db.Handles) model {
	loginInput := textinput.New()
	loginInput.Placeholder = "Wallet passphrase"
	loginInput.EchoMode = textinput.EchoPassword
	loginInput.EchoCharacter = '*'
	loginInput.Width = 40

	nameInput := textinput.New()
	nameInput.Placeholder = "Wallet name"
	nameInput.Width = 40

	secretInput := textinput.New()
	secretInput.Placeholder = "Mnemonic or passphrase"
	secretInput.Width = 64

	passInput := textinput.New()
	passInput.Placeholder = "Encryption passphrase"
	passInput.Width = 40
	passInput.EchoMode = textinput.EchoPassword
	passInput.EchoCharacter = '*'

	sendToInput := textinput.New()
	sendToInput.Placeholder = "Destination address"
	sendToInput.Width = 56

	amountInput := textinput.New()
	amountInput.Placeholder = "Amount"
	amountInput.Width = 20

	memoInput := textinput.New()
	memoInput.Placeholder = "Memo (optional)"
	memoInput.Width = 28

	loginInput.Focus()

	return model{
		store:          handles.Wallet,
		indexStore:     handles.Index,
		walletReadOnly: handles.WalletReadOnly,
		network:        handles.Wallet.CurrentNetwork(),
		mode:           modeLogin,
		loading:        true,
		loginInput:     loginInput,
		nameInput:      nameInput,
		secretInput:    secretInput,
		passInput:      passInput,
		sendToInput:    sendToInput,
		amountInput:    amountInput,
		memoInput:      memoInput,
	}
}

func (m model) Init() tea.Cmd {
	return m.loadWalletInventoryCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case walletInventoryMsg:
		m.loading = false
		if msg.Err != nil {
			if errors.Is(msg.Err, wallet.ErrWalletNotFound) {
				m.mode = modeWelcome
				m.errMessage = ""
				m.wallets = nil
				m.activeItem = nil
				return m, nil
			}
			m.errMessage = normalizeUIError(msg.Err).Error()
			return m, nil
		}
		m.wallets = msg.Items
		m.activeItem = msg.Active
		if len(m.wallets) == 0 {
			m.mode = modeWelcome
			return m, nil
		}
		if m.session == nil {
			m.mode = modeLogin
			m.loginInput.Focus()
			if m.activeItem != nil {
				m.status = fmt.Sprintf("Unlock %s", m.activeItem.DisplayLabel)
			}
		}
		return m, nil
	case loginMsg:
		m.loading = false
		if msg.Err != nil {
			m.errMessage = normalizeUIError(msg.Err).Error()
			return m, nil
		}
		m.session = &msg.Session
		active := walletItem{
			Wallet:       msg.Session.Wallet,
			Account:      msg.Session.Account,
			Active:       true,
			DisplayLabel: walletDisplayLabel(msg.Session.Wallet, msg.Session.Account),
		}
		m.activeItem = &active
		m.loginInput.SetValue("")
		m.errMessage = ""
		m.status = fmt.Sprintf("Unlocked %s", active.DisplayLabel)
		m.mode = modeHome
		return m, tea.Batch(m.loadWalletInventoryCmd(), m.refreshAccountCmd())
	case accountMsg:
		m.loading = false
		if msg.Err != nil {
			m.errMessage = normalizeUIError(msg.Err).Error()
			return m, nil
		}
		m.account = msg.Account
		m.errMessage = ""
		if m.status == "" {
			m.status = fmt.Sprintf("Refreshed %s", time.Now().Format("15:04:05"))
		}
		return m, nil
	case historyMsg:
		m.loading = false
		if msg.Err != nil {
			m.errMessage = normalizeUIError(msg.Err).Error()
			return m, nil
		}
		m.history = msg.Items
		m.errMessage = ""
		m.status = fmt.Sprintf("Loaded %d indexed entries", len(msg.Items))
		return m, nil
	case sendMsg:
		m.loading = false
		if msg.Err != nil {
			m.errMessage = normalizeUIError(msg.Err).Error()
			return m, nil
		}
		m.errMessage = ""
		if strings.TrimSpace(m.memoInput.Value()) == "" {
			m.status = fmt.Sprintf("Sent %s XLM. Suggestion: use memo next time.", m.amountInput.Value())
		} else {
			m.status = fmt.Sprintf("Sent %s XLM", m.amountInput.Value())
		}
		m.sendToInput.SetValue("")
		m.amountInput.SetValue("")
		m.memoInput.SetValue("")
		m.mode = modeHome
		return m, tea.Batch(m.refreshAccountCmd(), m.loadHistoryCmd())
	case fundMsg:
		m.loading = false
		if msg.Err != nil {
			m.errMessage = normalizeUIError(msg.Err).Error()
			return m, nil
		}
		m.errMessage = ""
		m.status = "Funded testnet account"
		return m, tea.Batch(m.refreshAccountCmd(), m.loadHistoryCmd())
	case proposalMsg:
		m.loading = false
		if msg.Err != nil {
			m.errMessage = normalizeUIError(msg.Err).Error()
			return m, nil
		}
		m.errMessage = ""
		m.status = fmt.Sprintf("Proposal saved: %s", msg.Proposal.ID)
		m.sendToInput.SetValue("")
		m.amountInput.SetValue("")
		m.memoInput.SetValue("")
		m.mode = modeHome
		return m, nil
	case monitorMsg:
		m.monitoring = msg.Snapshot
		if m.mode == modeMonitor {
			return m, tea.Tick(5*time.Second, func(time.Time) tea.Msg { return monitorMsg{Snapshot: metrics.SnapshotNow()} })
		}
		return m, nil
	case networkMsg:
		m.loading = false
		if msg.Err != nil {
			m.errMessage = normalizeUIError(msg.Err).Error()
			return m, nil
		}
		m.network = msg.Network
		m.errMessage = ""
		m.status = fmt.Sprintf("Network switched to %s", msg.Network)
		if m.session != nil {
			return m, tea.Batch(m.refreshAccountCmd(), m.loadHistoryCmd())
		}
		return m, nil
	case walletSavedMsg:
		m.loading = false
		if msg.Err != nil {
			m.errMessage = normalizeUIError(msg.Err).Error()
			return m, nil
		}
		m.session = &msg.Session
		active := walletItem{
			Wallet:       msg.Session.Wallet,
			Account:      msg.Session.Account,
			Active:       true,
			DisplayLabel: walletDisplayLabel(msg.Session.Wallet, msg.Session.Account),
		}
		m.activeItem = &active
		m.nameInput.SetValue("")
		m.secretInput.SetValue("")
		m.passInput.SetValue("")
		m.loginInput.SetValue("")
		m.errMessage = ""
		m.status = fmt.Sprintf("Active wallet: %s", active.DisplayLabel)
		m.mode = modeHome
		return m, tea.Batch(m.loadWalletInventoryCmd(), m.refreshAccountCmd())
	case clipboardMsg:
		if msg.Err != nil {
			m.errMessage = msg.Err.Error()
			return m, nil
		}
		m.errMessage = ""
		m.status = "Address copied to clipboard"
		return m, nil
	case dashboardMsg:
		if msg.Err != nil {
			m.errMessage = msg.Err.Error()
			return m, nil
		}
		m.errMessage = ""
		m.status = "Dashboard opened in browser"
		return m, nil
	case tea.KeyMsg:
		if msg.String() != "q" {
			m.quitArmed = false
		}
		switch m.mode {
		case modeLogin:
			return m.updateLogin(msg)
		case modeCreate:
			return m.updateCreate(msg)
		case modeImport:
			return m.updateImport(msg)
		case modeSend, modePropose:
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
	case "q":
		if m.quitArmed {
			return m, tea.Quit
		}
		m.quitArmed = true
		m.status = "Press q again to quit."
		return m, nil
	case "r":
		if m.mode == modeMonitor {
			return m, m.loadMonitorCmd()
		}
		if m.session == nil {
			return m, nil
		}
		m.loading = true
		return m, tea.Batch(m.refreshAccountCmd(), m.loadHistoryCmd())
	case "n":
		m.loading = true
		return m, m.toggleNetworkCmd()
	case "h":
		if m.session == nil {
			return m, nil
		}
		m.mode = modeHistory
		m.loading = true
		return m, m.loadHistoryCmd()
	case "s":
		if m.session == nil {
			return m, nil
		}
		m.mode = modeSend
		m.focusIndex = 0
		m.focusSendInput()
		return m, nil
	case "p":
		if m.session == nil {
			return m, nil
		}
		m.mode = modePropose
		m.focusIndex = 0
		m.focusSendInput()
		return m, nil
	case "c":
		m.mode = modeSettings
		return m, nil
	case "m":
		metrics.RecordWalletAction("watch", "tui monitor")
		m.mode = modeMonitor
		return m, m.loadMonitorCmd()
	case "w":
		m.mode = modeWallets
		m.loading = true
		return m, m.loadWalletInventoryCmd()
	case "f":
		if m.session == nil || m.network != stellar.NetworkTestnet {
			return m, nil
		}
		m.loading = true
		return m, m.fundCmd()
	case "y":
		if m.session == nil {
			return m, nil
		}
		return m, copyAddressCmd(m.session.Account.Address)
	case "o":
		if m.mode == modeMonitor {
			return m, openDashboardCmd()
		}
	case "i":
		if m.mode == modeWelcome || m.mode == modeWallets {
			if m.walletReadOnly {
				m.errMessage = "Database is in use by another process. Close CLI or reopen TUI with write access."
				return m, nil
			}
			m.mode = modeImport
			m.focusImportInput()
			return m, nil
		}
	case "a":
		if m.mode == modeWelcome || m.mode == modeWallets {
			if m.walletReadOnly {
				m.errMessage = "Database is in use by another process. Close CLI or reopen TUI with write access."
				return m, nil
			}
			m.mode = modeCreate
			m.focusCreateInput()
			return m, nil
		}
	case "l":
		if m.session != nil {
			m.session = nil
			m.mode = modeLogin
			m.status = "Locked. Enter passphrase."
			m.loginInput.Focus()
			return m, nil
		}
	case "esc":
		if m.session != nil {
			m.mode = modeHome
		}
		return m, nil
	}
	return m, nil
}

func (m model) updateLogin(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		if m.quitArmed {
			return m, tea.Quit
		}
		m.quitArmed = true
		m.status = "Press q again to quit."
		return m, nil
	case "enter":
		m.loading = true
		m.errMessage = ""
		return m, m.unlockWalletCmd(m.selectedOrActiveItem(), m.loginInput.Value())
	case "a":
		m.mode = modeCreate
		m.focusCreateInput()
		return m, nil
	case "i":
		m.mode = modeImport
		m.focusImportInput()
		return m, nil
	}
	var cmd tea.Cmd
	m.loginInput, cmd = m.loginInput.Update(msg)
	return m, cmd
}

func (m model) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = returnMode(m.session)
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.cycleCreateFocus(msg.String())
		return m, nil
	case "enter":
		m.loading = true
		m.errMessage = ""
		return m, m.createWalletCmd(m.nameInput.Value(), m.passInput.Value())
	}
	var cmd tea.Cmd
	if m.focusIndex == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.passInput, cmd = m.passInput.Update(msg)
	}
	return m, cmd
}

func (m model) updateImport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = returnMode(m.session)
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.cycleImportFocus(msg.String())
		return m, nil
	case "enter":
		m.loading = true
		m.errMessage = ""
		return m, m.importWalletCmd(m.nameInput.Value(), m.secretInput.Value(), m.passInput.Value())
	}
	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.secretInput, cmd = m.secretInput.Update(msg)
	case 2:
		m.passInput, cmd = m.passInput.Update(msg)
	}
	return m, cmd
}

func (m model) updateSend(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = modeHome
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.cycleSendFocus(msg.String())
		return m, nil
	case "enter":
		m.loading = true
		m.errMessage = ""
		if m.mode == modePropose {
			return m, m.proposeCmd()
		}
		return m, m.sendCmd()
	}
	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.sendToInput, cmd = m.sendToInput.Update(msg)
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
		if len(m.wallets) > 0 {
			m.walletCursor--
			if m.walletCursor < 0 {
				m.walletCursor = len(m.wallets) - 1
			}
		}
		return m, nil
	case "down", "j":
		if len(m.wallets) > 0 {
			m.walletCursor++
			if m.walletCursor >= len(m.wallets) {
				m.walletCursor = 0
			}
		}
		return m, nil
	case "enter":
		selected, ok := m.selectedWallet()
		if !ok {
			return m, nil
		}
		if selected.Active {
			m.status = "Wallet already active"
			return m, nil
		}
		m.mode = modeLogin
		m.activeItem = &selected
		m.loginInput.SetValue("")
		m.loginInput.Focus()
		m.status = fmt.Sprintf("Enter passphrase for %s", selected.DisplayLabel)
		return m, nil
	case "a":
		m.mode = modeCreate
		m.focusCreateInput()
		return m, nil
	case "i":
		m.mode = modeImport
		m.focusImportInput()
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).Render("Nebula")
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	errorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)

	footer := statusStyle.Render(m.status)
	if m.errMessage != "" {
		footer = errorStyle.Render(m.errMessage)
	}
	if m.loading {
		footer = "Loading..."
	}

	content := []string{title, ""}
	switch m.mode {
	case modeWelcome:
		content = append(content, m.welcomeView())
	case modeLogin:
		content = append(content, m.loginView())
	case modeHome:
		content = append(content, m.homeView())
	case modeSend:
		content = append(content, m.homeView(), "", m.sendView("Send"))
	case modePropose:
		content = append(content, m.homeView(), "", m.sendView("Propose Multisig Tx"))
	case modeHistory:
		content = append(content, m.homeView(), "", m.historyView())
	case modeSettings:
		content = append(content, m.homeView(), "", m.settingsView())
	case modeMonitor:
		content = append(content, m.homeView(), "", m.monitorView())
	case modeWallets:
		content = append(content, m.homeView(), "", m.walletsView())
	case modeCreate:
		content = append(content, m.createView())
	case modeImport:
		content = append(content, m.importView())
	}
	content = append(content, "", footer)
	return lipgloss.NewStyle().Padding(1, 2).Render(strings.Join(content, "\n"))
}

func (m model) welcomeView() string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1)
	return box.Render(strings.Join([]string{
		"Welcome",
		"",
		"No wallets are configured yet.",
		"Press [a] to create a wallet.",
		"Press [i] to import a wallet.",
		"Press [q] twice to quit.",
	}, "\n"))
}

func (m model) loginView() string {
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1)
	target := "active wallet"
	if m.activeItem != nil {
		target = m.activeItem.DisplayLabel
	}
	lines := []string{
		"Login",
		"",
		"Unlock " + target,
		m.loginInput.View(),
		"",
		"Enter unlocks. [a] create, [i] import, [q] twice quits.",
	}
	return box.Render(strings.Join(lines, "\n"))
}

func (m model) homeView() string {
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1)
	actions := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(1)
	name := "Locked"
	address := "No wallet"
	path := "n/a"
	if m.session != nil {
		name = m.session.Wallet.Name
		address = m.session.Account.Address
		path = m.session.Account.Path
	}
	balanceText := "Account not funded"
	if m.account.Funded {
		balanceText = fmt.Sprintf("%s XLM", m.account.Balance)
	}
	left := panel.Render(strings.Join([]string{
		"Wallet",
		"",
		fmt.Sprintf("Name: %s", name),
		fmt.Sprintf("Address: %s", address),
		fmt.Sprintf("Path: %s", path),
		fmt.Sprintf("Balance: %s", balanceText),
		fmt.Sprintf("Network: %s", m.network),
		fmt.Sprintf("Reserve: %s XLM", m.account.Reserve),
	}, "\n"))
	right := actions.Render(strings.Join([]string{
		"Actions",
		"",
		"[s] Send",
		"[p] Propose tx",
		"[h] History",
		"[m] Monitor",
		"[r] Refresh",
		"[w] Wallets",
		"[n] Toggle network",
		"[c] Settings",
		"[f] Friendbot",
		"[y] Copy address",
		"[l] Lock",
		"[q] Quit",
	}, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, right)
}

func (m model) monitorView() string {
	return tuiview.RenderMonitoringPanel(m.monitoring, monitoring.GrafanaURL)
}

func (m model) sendView(title string) string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	description := "Enter submits live payment. Tab moves fields. Esc returns home."
	if m.mode == modePropose {
		description = "Enter saves unsigned multisig proposal. Tab moves fields. Esc returns home."
	}
	return box.Render(strings.Join([]string{
		title,
		"",
		m.sendToInput.View(),
		m.amountInput.View(),
		m.memoInput.View(),
		"",
		description,
	}, "\n"))
}

func (m model) historyView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	if len(m.history) == 0 {
		return box.Render("History\n\nNo indexed transactions yet. Press [h] after refresh or sync activity.")
	}
	lines := []string{"History", ""}
	for _, item := range m.history {
		lines = append(lines, fmt.Sprintf("%s  %s  %s %s %s  %s",
			item.Timestamp.Format("2006-01-02 15:04"),
			shortHash(item.Hash),
			item.Direction,
			item.Amount,
			item.AssetCode,
			shortAddress(item.Counterparty),
		))
		lines = append(lines, "  "+item.ExplorerURL)
	}
	return box.Render(strings.Join(lines, "\n"))
}

func (m model) settingsView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	proposalPath := filepath.Join(m.store.RootDir(), "proposals")
	lines := []string{
		"Settings",
		"",
		fmt.Sprintf("Storage: %s", m.store.RootDir()),
		fmt.Sprintf("Wallet DB: %s", m.store.DBDir()),
		fmt.Sprintf("Index DB: %s", m.store.IndexDir()),
		fmt.Sprintf("Wallet mode: %s", walletModeLabel(m.walletReadOnly)),
		fmt.Sprintf("Proposals: %s", proposalPath),
		"",
		"Install globally:",
		"  go install ./cmd/nb ./cmd/nbtui",
		"  or place binaries in ~/.local/bin and add it to PATH",
		"",
		"Press [esc] to return home.",
	}
	return box.Render(strings.Join(lines, "\n"))
}

func (m model) walletsView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	if len(m.wallets) == 0 {
		return box.Render("Wallets\n\nNo wallets saved.")
	}
	lines := []string{"Wallets", "", fmt.Sprintf("Saved in: %s", m.store.DBDir()), ""}
	for i, item := range m.wallets {
		cursor := " "
		if i == m.walletCursor {
			cursor = ">"
		}
		status := "saved"
		if item.Active {
			status = "active"
		}
		lines = append(lines, fmt.Sprintf("%s %s  %s", cursor, item.DisplayLabel, status))
		lines = append(lines, "  "+item.Account.Address)
	}
	lines = append(lines, "", "Enter prepares login for selected wallet account. [a] create, [i] import.")
	return box.Render(strings.Join(lines, "\n"))
}

func (m model) createView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	return box.Render(strings.Join([]string{
		"Create Wallet",
		"",
		m.nameInput.View(),
		m.passInput.View(),
		"",
		"Name, then encryption passphrase. Enter creates wallet. Esc cancels.",
	}, "\n"))
}

func (m model) importView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	return box.Render(strings.Join([]string{
		"Import Wallet",
		"",
		m.nameInput.View(),
		m.secretInput.View(),
		m.passInput.View(),
		"",
		"Name, mnemonic, then encryption passphrase. Enter imports. Esc cancels.",
	}, "\n"))
}

func (m model) loadWalletInventoryCmd() tea.Cmd {
	store := m.store
	return func() tea.Msg {
		items, active, err := loadWalletItems(store)
		return walletInventoryMsg{Items: items, Active: active, Err: err}
	}
}

func (m model) refreshAccountCmd() tea.Cmd {
	if m.session == nil {
		return nil
	}
	current := *m.session
	networkName := m.network
	return func() tea.Msg {
		client, err := stellar.NewClient(networkName)
		if err != nil {
			return accountMsg{Err: err}
		}
		balance, funded, err := client.Balance(current.Account.Address)
		if err != nil {
			return accountMsg{Err: err}
		}
		return accountMsg{Account: accountState{
			Address: current.Account.Address,
			Balance: balance,
			Funded:  funded,
			Reserve: stellar.FormatStroops(10_000_000),
		}}
	}
}

func (m model) loadHistoryCmd() tea.Cmd {
	if m.session == nil {
		return nil
	}
	current := *m.session
	idx := m.indexStore
	return func() tea.Msg {
		if idx == nil {
			return historyMsg{Err: fmt.Errorf("index database is unavailable")}
		}
		items, err := idx.SearchAccount(current.Account.Address, 365*24*time.Hour)
		if err != nil {
			return historyMsg{Err: err}
		}
		if len(items) > 10 {
			items = items[:10]
		}
		return historyMsg{Items: items}
	}
}

func (m model) loadMonitorCmd() tea.Cmd {
	return func() tea.Msg {
		return monitorMsg{Snapshot: metrics.SnapshotNow()}
	}
}

func (m model) sendCmd() tea.Cmd {
	if m.session == nil {
		return nil
	}
	current := *m.session
	networkName := m.network
	address := m.sendToInput.Value()
	amount := m.amountInput.Value()
	memo := m.memoInput.Value()
	return func() tea.Msg {
		client, err := stellar.NewClient(networkName)
		if err != nil {
			return sendMsg{Err: err}
		}
		hash, err := client.SendPayment(current.Secret, address, amount, memo)
		return sendMsg{Hash: hash, Err: err}
	}
}

func (m model) proposeCmd() tea.Cmd {
	if m.session == nil {
		return nil
	}
	current := *m.session
	networkName := m.network
	address := m.sendToInput.Value()
	amount := m.amountInput.Value()
	memo := m.memoInput.Value()
	store := m.store
	return func() tea.Msg {
		service := multisig.NewService(store)
		proposal, err := service.ProposePayment(current.Secret, networkName, current.Wallet.ID, current.Account.Index, address, amount, memo)
		return proposalMsg{Proposal: proposal, Err: err}
	}
}

func (m model) fundCmd() tea.Cmd {
	if m.session == nil {
		return nil
	}
	current := *m.session
	networkName := m.network
	return func() tea.Msg {
		client, err := stellar.NewClient(networkName)
		if err != nil {
			return fundMsg{Err: err}
		}
		hash, err := client.FundTestnet(current.Account.Address)
		return fundMsg{Hash: hash, Err: err}
	}
}

func (m model) toggleNetworkCmd() tea.Cmd {
	store := m.store
	return func() tea.Msg {
		networkName, err := store.ToggleNetwork()
		return networkMsg{Network: networkName, Err: err}
	}
}

func (m model) unlockWalletCmd(item *walletItem, passphrase string) tea.Cmd {
	store := m.store
	readonly := m.walletReadOnly
	return func() tea.Msg {
		if item == nil {
			return loginMsg{Err: wallet.ErrWalletNotFound}
		}
		if readonly && !item.Active {
			return loginMsg{Err: fmt.Errorf("database is in use by another process. Close CLI or reopen TUI with write access")}
		}
		if !item.Active {
			if err := store.SetActiveWallet(item.Wallet.ID, item.Account.Index); err != nil {
				return loginMsg{Err: err}
			}
		}
		summary, account, secret, err := store.UnlockAccount(item.Wallet.ID, item.Account.Index, passphrase)
		if err != nil {
			return loginMsg{Err: err}
		}
		return loginMsg{Session: session{
			Wallet:     summary,
			Account:    account,
			Secret:     secret,
			Passphrase: passphrase,
		}}
	}
}

func (m model) createWalletCmd(name, passphrase string) tea.Cmd {
	store := m.store
	if m.walletReadOnly {
		return func() tea.Msg {
			return walletSavedMsg{Err: fmt.Errorf("database is in use by another process. Close CLI or reopen TUI with write access")}
		}
	}
	return func() tea.Msg {
		summary, _, err := store.CreateWallet(wallet.CreateOptions{Name: name, Passphrase: passphrase, Words: 24})
		if err != nil {
			return walletSavedMsg{Err: err}
		}
		if err := store.SetActiveWallet(summary.ID, 0); err != nil {
			return walletSavedMsg{Err: err}
		}
		unlockedSummary, account, secret, err := store.ActiveAccount(passphrase)
		return walletSavedMsg{Session: session{
			Wallet:     unlockedSummary,
			Account:    account,
			Secret:     secret,
			Passphrase: passphrase,
		}, Err: err}
	}
}

func (m model) importWalletCmd(name, mnemonic, passphrase string) tea.Cmd {
	store := m.store
	if m.walletReadOnly {
		return func() tea.Msg {
			return walletSavedMsg{Err: fmt.Errorf("database is in use by another process. Close CLI or reopen TUI with write access")}
		}
	}
	return func() tea.Msg {
		summary, err := store.ImportWallet(wallet.ImportOptions{Name: name, Mnemonic: mnemonic, Passphrase: passphrase})
		if err != nil {
			return walletSavedMsg{Err: err}
		}
		if err := store.SetActiveWallet(summary.ID, 0); err != nil {
			return walletSavedMsg{Err: err}
		}
		unlockedSummary, account, secret, err := store.ActiveAccount(passphrase)
		return walletSavedMsg{Session: session{
			Wallet:     unlockedSummary,
			Account:    account,
			Secret:     secret,
			Passphrase: passphrase,
		}, Err: err}
	}
}

func copyAddressCmd(address string) tea.Cmd {
	return func() tea.Msg {
		return clipboardMsg{Err: clipboard.WriteAll(address)}
	}
}

func openDashboardCmd() tea.Cmd {
	return func() tea.Msg {
		return dashboardMsg{Err: monitoring.OpenBrowser(monitoring.GrafanaURL)}
	}
}

func (m *model) focusCreateInput() {
	m.focusIndex = 0
	m.nameInput.SetValue("")
	m.passInput.SetValue("")
	m.nameInput.Placeholder = "Wallet name"
	m.passInput.Placeholder = "Wallet passphrase"
	m.nameInput.Focus()
	m.passInput.Blur()
	m.secretInput.Blur()
	m.loginInput.Blur()
}

func (m *model) focusImportInput() {
	m.focusIndex = 0
	m.nameInput.SetValue("")
	m.secretInput.SetValue("")
	m.passInput.SetValue("")
	m.nameInput.Placeholder = "Wallet name"
	m.secretInput.Placeholder = "Mnemonic phrase"
	m.nameInput.Focus()
	m.secretInput.Blur()
	m.passInput.Blur()
}

func (m *model) focusSendInput() {
	m.sendToInput.Focus()
	m.amountInput.Blur()
	m.memoInput.Blur()
}

func (m *model) cycleCreateFocus(direction string) {
	if direction == "shift+tab" || direction == "up" {
		m.focusIndex--
	} else {
		m.focusIndex++
	}
	if m.focusIndex < 0 {
		m.focusIndex = 1
	}
	if m.focusIndex > 1 {
		m.focusIndex = 0
	}
	if m.focusIndex == 0 {
		m.nameInput.Focus()
		m.passInput.Blur()
	} else {
		m.nameInput.Blur()
		m.passInput.Focus()
	}
}

func (m *model) cycleImportFocus(direction string) {
	if direction == "shift+tab" || direction == "up" {
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
	m.nameInput.Blur()
	m.secretInput.Blur()
	m.passInput.Blur()
	switch m.focusIndex {
	case 0:
		m.nameInput.Focus()
	case 1:
		m.secretInput.Focus()
	case 2:
		m.passInput.Focus()
	}
}

func (m *model) cycleSendFocus(direction string) {
	if direction == "shift+tab" || direction == "up" {
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
	m.sendToInput.Blur()
	m.amountInput.Blur()
	m.memoInput.Blur()
	switch m.focusIndex {
	case 0:
		m.sendToInput.Focus()
	case 1:
		m.amountInput.Focus()
	case 2:
		m.memoInput.Focus()
	}
}

func (m model) selectedWallet() (walletItem, bool) {
	if len(m.wallets) == 0 || m.walletCursor < 0 || m.walletCursor >= len(m.wallets) {
		return walletItem{}, false
	}
	return m.wallets[m.walletCursor], true
}

func (m model) selectedOrActiveItem() *walletItem {
	if m.activeItem != nil {
		return m.activeItem
	}
	if len(m.wallets) == 0 {
		return nil
	}
	item := m.wallets[0]
	return &item
}

func loadWalletItems(store *wallet.Store) ([]walletItem, *walletItem, error) {
	summaries, err := store.ListWallets()
	if err != nil {
		return nil, nil, err
	}
	cfg, err := store.Config()
	if err != nil {
		return nil, nil, err
	}
	items := []walletItem{}
	var active *walletItem
	for _, summary := range summaries {
		for _, account := range summary.Accounts {
			item := walletItem{
				Wallet:       summary,
				Account:      account,
				Active:       summary.ID == cfg.ActiveWalletID && account.Index == cfg.ActiveAccountIndex,
				DisplayLabel: walletDisplayLabel(summary, account),
			}
			items = append(items, item)
			if item.Active {
				copy := item
				active = &copy
			}
		}
	}
	return items, active, nil
}

func walletDisplayLabel(summary wallet.WalletSummary, account wallet.DerivedAccount) string {
	return fmt.Sprintf("%s [%d]", summary.Name, account.Index)
}

func normalizeUIError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, wallet.ErrWalletNotFound):
		return fmt.Errorf("wallet not found. Create or import one first")
	case errors.Is(err, wallet.ErrInvalidPassphrase):
		return fmt.Errorf("invalid passphrase")
	case errors.Is(err, wallet.ErrInvalidMnemonic):
		return fmt.Errorf("invalid mnemonic")
	case errors.Is(err, stellar.ErrAccountNotFunded):
		return fmt.Errorf("Account not funded. Use Friendbot on testnet")
	case errors.Is(err, stellar.ErrInvalidAddress):
		return fmt.Errorf("invalid address")
	case errors.Is(err, stellar.ErrInvalidAmount):
		return fmt.Errorf("amount must be greater than 0")
	case errors.Is(err, stellar.ErrInsufficientBalance):
		return fmt.Errorf("insufficient balance after reserve")
	case errors.Is(err, stellar.ErrFriendbotLimit):
		return fmt.Errorf("Friendbot limit reached")
	default:
		return err
	}
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

func returnMode(current *session) viewMode {
	if current == nil {
		return modeWelcome
	}
	return modeHome
}

func walletModeLabel(readOnly bool) string {
	if readOnly {
		return "read-only"
	}
	return "read-write"
}
