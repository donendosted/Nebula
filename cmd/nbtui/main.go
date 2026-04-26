package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"nebula/nebula"

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
	modeHistory  viewMode = "history"
	modeSettings viewMode = "settings"
	modeWallets  viewMode = "wallets"
	modeCreate   viewMode = "create"
	modeImport   viewMode = "import"
)

type walletInventoryMsg struct {
	wallets []nebula.WalletMeta
	active  *nebula.WalletMeta
	err     error
}

type loginMsg struct {
	unlocked nebula.UnlockedWallet
	err      error
}

type accountMsg struct {
	info nebula.AccountInfo
	err  error
}

type historyMsg struct {
	items []nebula.HistoryEntry
	err   error
}

type sendMsg struct {
	result nebula.SendResult
	err    error
}

type fundMsg struct {
	result nebula.FundResult
	err    error
}

type networkMsg struct {
	network nebula.Network
	err     error
}

type walletSavedMsg struct {
	unlocked nebula.UnlockedWallet
	err      error
}

type clipboardMsg struct {
	err error
}

type model struct {
	store   *nebula.Store
	network nebula.Network
	mode    viewMode

	activeWallet *nebula.WalletMeta
	unlocked     *nebula.UnlockedWallet
	wallets      []nebula.WalletMeta
	history      []nebula.HistoryEntry
	account      nebula.AccountInfo

	loading    bool
	status     string
	errMessage string
	quitArmed  bool

	loginInput  textinput.Model
	nameInput   textinput.Model
	secretInput textinput.Model
	sendToInput textinput.Model
	amountInput textinput.Model
	memoInput   textinput.Model
	focusIndex  int

	walletCursor int
	width        int
	height       int
}

func main() {
	store, err := nebula.NewStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	networkValue, err := store.CurrentNetwork()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	m := newModel(store, networkValue)
	if _, err := tea.NewProgram(m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newModel(store *nebula.Store, networkValue nebula.Network) model {
	loginInput := textinput.New()
	loginInput.Placeholder = "Wallet passphrase"
	loginInput.EchoMode = textinput.EchoPassword
	loginInput.EchoCharacter = '*'
	loginInput.Width = 40

	nameInput := textinput.New()
	nameInput.Placeholder = "Wallet name"
	nameInput.Width = 40

	secretInput := textinput.New()
	secretInput.Placeholder = "Secret seed or passphrase"
	secretInput.Width = 64

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
		store:       store,
		network:     networkValue,
		mode:        modeLogin,
		loading:     true,
		loginInput:  loginInput,
		nameInput:   nameInput,
		secretInput: secretInput,
		sendToInput: sendToInput,
		amountInput: amountInput,
		memoInput:   memoInput,
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
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			if errors.Is(msg.err, nebula.ErrWalletNotFound) {
				m.mode = modeWelcome
				m.errMessage = ""
			}
			return m, nil
		}
		m.wallets = msg.wallets
		m.activeWallet = msg.active
		if len(m.wallets) == 0 {
			m.mode = modeWelcome
			return m, nil
		}
		if m.unlocked == nil {
			m.mode = modeLogin
			m.loginInput.Focus()
			m.status = fmt.Sprintf("Unlock %s", m.activeWallet.Name)
		}
		return m, nil
	case loginMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = normalizeUIError(msg.err).Error()
			return m, nil
		}
		m.unlocked = &msg.unlocked
		active := msg.unlocked.Meta
		m.activeWallet = &active
		m.loginInput.SetValue("")
		m.errMessage = ""
		m.status = fmt.Sprintf("Unlocked %s", active.Name)
		m.mode = modeHome
		return m, tea.Batch(m.loadWalletInventoryCmd(), m.refreshAccountCmd())
	case accountMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = normalizeUIError(msg.err).Error()
			return m, nil
		}
		m.account = msg.info
		if m.unlocked != nil {
			m.account.Name = m.unlocked.Meta.Name
		}
		m.errMessage = ""
		if m.status == "" {
			m.status = fmt.Sprintf("Refreshed %s", time.Now().Format("15:04:05"))
		}
		return m, nil
	case historyMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = normalizeUIError(msg.err).Error()
			return m, nil
		}
		m.history = msg.items
		m.errMessage = ""
		m.status = fmt.Sprintf("Loaded %d entries", len(msg.items))
		return m, nil
	case sendMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = normalizeUIError(msg.err).Error()
			return m, nil
		}
		m.errMessage = ""
		if strings.TrimSpace(m.memoInput.Value()) == "" {
			m.status = fmt.Sprintf("Sent %s %s. Suggestion: use memo next time.", msg.result.Amount, msg.result.AssetCode)
		} else {
			m.status = fmt.Sprintf("Sent %s %s", msg.result.Amount, msg.result.AssetCode)
		}
		m.sendToInput.SetValue("")
		m.amountInput.SetValue("")
		m.memoInput.SetValue("")
		m.mode = modeHome
		return m, tea.Batch(m.refreshAccountCmd(), m.loadHistoryCmd())
	case fundMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = normalizeUIError(msg.err).Error()
			return m, nil
		}
		if msg.result.LimitReached {
			m.status = "Limit reached"
		} else {
			m.status = fmt.Sprintf("Funded %d/2 times", msg.result.FundedCount)
		}
		m.errMessage = ""
		return m, tea.Batch(m.refreshAccountCmd(), m.loadWalletInventoryCmd())
	case networkMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = normalizeUIError(msg.err).Error()
			return m, nil
		}
		m.network = msg.network
		m.errMessage = ""
		m.status = fmt.Sprintf("Network switched to %s", msg.network)
		if m.unlocked != nil {
			return m, tea.Batch(m.refreshAccountCmd(), m.loadHistoryCmd())
		}
		return m, nil
	case walletSavedMsg:
		m.loading = false
		if msg.err != nil {
			m.errMessage = normalizeUIError(msg.err).Error()
			return m, nil
		}
		m.unlocked = &msg.unlocked
		active := msg.unlocked.Meta
		m.activeWallet = &active
		m.nameInput.SetValue("")
		m.secretInput.SetValue("")
		m.loginInput.SetValue("")
		m.errMessage = ""
		m.status = fmt.Sprintf("Active wallet: %s", active.Name)
		m.mode = modeHome
		return m, tea.Batch(m.loadWalletInventoryCmd(), m.refreshAccountCmd())
	case clipboardMsg:
		if msg.err != nil {
			m.errMessage = msg.err.Error()
			return m, nil
		}
		m.errMessage = ""
		m.status = "Address copied to clipboard"
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
	case "q":
		if m.quitArmed {
			return m, tea.Quit
		}
		m.quitArmed = true
		m.status = "Press q again to quit."
		return m, nil
	case "r":
		if m.unlocked == nil {
			return m, nil
		}
		m.loading = true
		return m, m.refreshAccountCmd()
	case "n":
		m.loading = true
		return m, m.toggleNetworkCmd()
	case "h":
		if m.unlocked == nil {
			return m, nil
		}
		m.mode = modeHistory
		m.loading = true
		return m, m.loadHistoryCmd()
	case "s":
		if m.unlocked == nil {
			return m, nil
		}
		m.mode = modeSend
		m.focusIndex = 0
		m.focusSendInput()
		return m, nil
	case "c":
		m.mode = modeSettings
		return m, nil
	case "w":
		m.mode = modeWallets
		m.loading = true
		return m, m.loadWalletInventoryCmd()
	case "f":
		if m.unlocked == nil || m.network != nebula.NetworkTestnet {
			return m, nil
		}
		m.loading = true
		return m, m.fundCmd()
	case "y":
		if m.unlocked == nil {
			return m, nil
		}
		return m, copyAddressCmd(m.unlocked.Meta.Address)
	case "i":
		if m.mode == modeWelcome || m.mode == modeWallets {
			m.mode = modeImport
			m.focusImportInput()
			return m, nil
		}
	case "a":
		if m.mode == modeWelcome || m.mode == modeWallets {
			m.mode = modeCreate
			m.focusCreateInput()
			return m, nil
		}
	case "l":
		if m.unlocked != nil {
			m.unlocked = nil
			m.mode = modeLogin
			m.status = "Locked. Enter passphrase."
			m.loginInput.Focus()
			return m, nil
		}
	case "esc":
		if m.unlocked != nil {
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
		target := ""
		activate := false
		if m.activeWallet != nil {
			target = m.activeWallet.Address
			if m.unlocked == nil || m.unlocked.Meta.Address != m.activeWallet.Address {
				activate = true
			}
		}
		m.loading = true
		m.errMessage = ""
		return m, m.unlockWalletCmd(target, m.loginInput.Value(), activate)
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
		m.mode = returnMode(m.unlocked)
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.cycleCreateFocus(msg.String())
		return m, nil
	case "enter":
		m.loading = true
		m.errMessage = ""
		return m, m.createWalletCmd(m.nameInput.Value(), m.secretInput.Value())
	}
	var cmd tea.Cmd
	if m.focusIndex == 0 {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.secretInput, cmd = m.secretInput.Update(msg)
	}
	return m, cmd
}

func (m model) updateImport(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.mode = returnMode(m.unlocked)
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.cycleImportFocus(msg.String())
		return m, nil
	case "enter":
		m.loading = true
		m.errMessage = ""
		return m, m.importWalletCmd(m.nameInput.Value(), m.secretInput.Value(), m.loginInput.Value())
	}
	var cmd tea.Cmd
	switch m.focusIndex {
	case 0:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case 1:
		m.secretInput, cmd = m.secretInput.Update(msg)
	case 2:
		m.loginInput, cmd = m.loginInput.Update(msg)
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
		m.activeWallet = &selected
		m.loginInput.SetValue("")
		m.loginInput.Focus()
		m.status = fmt.Sprintf("Enter passphrase for %s", selected.Name)
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
		content = append(content, m.homeView(), "", m.sendView())
	case modeHistory:
		content = append(content, m.homeView(), "", m.historyView())
	case modeSettings:
		content = append(content, m.homeView(), "", m.settingsView())
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
	if m.activeWallet != nil {
		target = fmt.Sprintf("%s (%s)", m.activeWallet.Name, shortAddress(m.activeWallet.Address))
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
	if m.unlocked != nil {
		name = m.unlocked.Meta.Name
		address = m.unlocked.Meta.Address
	}
	balanceText := "Account not funded"
	if m.account.Funded {
		balanceText = fmt.Sprintf("%s %s", nativeBalance(m.account), nebula.AssetCodeXLM)
	}
	left := panel.Render(strings.Join([]string{
		"Wallet",
		"",
		fmt.Sprintf("Name: %s", name),
		fmt.Sprintf("Address: %s", address),
		fmt.Sprintf("Balance: %s", balanceText),
		fmt.Sprintf("Network: %s", m.network),
		fmt.Sprintf("Reserve: %s XLM", m.account.Reserve),
	}, "\n"))
	right := actions.Render(strings.Join([]string{
		"Actions",
		"",
		"[s] Send",
		"[h] History",
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

func (m model) sendView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	return box.Render(strings.Join([]string{
		"Send",
		"",
		m.sendToInput.View(),
		m.amountInput.View(),
		m.memoInput.View(),
		"",
		"Enter submits. Tab moves fields. Esc returns home.",
	}, "\n"))
}

func (m model) historyView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	if len(m.history) == 0 {
		return box.Render("History\n\nNo recent transactions.")
	}
	lines := []string{"History", ""}
	for _, item := range m.history {
		lines = append(lines, fmt.Sprintf("%s  %s  %s %s %s  %s",
			item.CreatedAt.Format("2006-01-02 15:04"),
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
	activePath := "Locked"
	if path, err := m.store.ActiveWalletPath(); err == nil {
		activePath = path
	}
	lines := []string{
		"Settings",
		"",
		fmt.Sprintf("Storage: %s", m.store.BaseDir()),
		fmt.Sprintf("Wallets: %s", m.store.WalletsDir()),
		fmt.Sprintf("Config: %s", m.store.ConfigPath()),
		fmt.Sprintf("Active secret: %s", activePath),
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
	lines := []string{"Wallets", "", fmt.Sprintf("Saved in: %s", m.store.WalletsDir()), ""}
	for i, item := range m.wallets {
		cursor := " "
		if i == m.walletCursor {
			cursor = ">"
		}
		status := "saved"
		if item.Active {
			status = "active"
		}
		lines = append(lines, fmt.Sprintf("%s %s  %s  %s", cursor, item.Name, shortAddress(item.Address), status))
		lines = append(lines, "  "+item.SecretPath)
	}
	lines = append(lines, "", "Enter prepares login for selected wallet. [a] create, [i] import.")
	return box.Render(strings.Join(lines, "\n"))
}

func (m model) createView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	return box.Render(strings.Join([]string{
		"Create Wallet",
		"",
		m.nameInput.View(),
		m.secretInput.View(),
		"",
		"Passphrase input above is hidden. Enter creates wallet. Esc cancels.",
	}, "\n"))
}

func (m model) importView() string {
	box := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(1)
	return box.Render(strings.Join([]string{
		"Import Wallet",
		"",
		m.nameInput.View(),
		m.secretInput.View(),
		m.loginInput.View(),
		"",
		"Name, secret seed, then passphrase. Enter imports. Esc cancels.",
	}, "\n"))
}

func (m model) loadWalletInventoryCmd() tea.Cmd {
	store := m.store
	return func() tea.Msg {
		wallets, err := store.ListWallets()
		if err != nil {
			return walletInventoryMsg{err: err}
		}
		active, err := store.ActiveWallet()
		if err != nil {
			return walletInventoryMsg{wallets: wallets, err: err}
		}
		return walletInventoryMsg{wallets: wallets, active: &active}
	}
}

func (m model) refreshAccountCmd() tea.Cmd {
	if m.unlocked == nil {
		return nil
	}
	unlocked := *m.unlocked
	networkValue := m.network
	return func() tea.Msg {
		client, err := unlocked.Client(networkValue)
		if err != nil {
			return accountMsg{err: err}
		}
		info, err := client.Balance()
		if err != nil {
			return accountMsg{err: err}
		}
		info.Name = unlocked.Meta.Name
		return accountMsg{info: info}
	}
}

func (m model) loadHistoryCmd() tea.Cmd {
	if m.unlocked == nil {
		return nil
	}
	unlocked := *m.unlocked
	networkValue := m.network
	return func() tea.Msg {
		client, err := unlocked.Client(networkValue)
		if err != nil {
			return historyMsg{err: err}
		}
		items, err := client.History(nebula.DefaultHistoryLimit)
		return historyMsg{items: items, err: err}
	}
}

func (m model) sendCmd() tea.Cmd {
	if m.unlocked == nil {
		return nil
	}
	unlocked := *m.unlocked
	networkValue := m.network
	address := m.sendToInput.Value()
	amount := m.amountInput.Value()
	memo := m.memoInput.Value()
	return func() tea.Msg {
		client, err := unlocked.Client(networkValue)
		if err != nil {
			return sendMsg{err: err}
		}
		result, err := client.Send(address, amount, memo)
		return sendMsg{result: result, err: err}
	}
}

func (m model) fundCmd() tea.Cmd {
	if m.unlocked == nil {
		return nil
	}
	unlocked := *m.unlocked
	store := m.store
	networkValue := m.network
	return func() tea.Msg {
		client, err := unlocked.Client(networkValue)
		if err != nil {
			return fundMsg{err: err}
		}
		hash, err := client.FundTestnet()
		if err != nil {
			if errors.Is(err, nebula.ErrFriendbotLimit) {
				return fundMsg{result: nebula.FundResult{LimitReached: true}}
			}
			return fundMsg{err: err}
		}
		count, err := store.RecordTestnetFunding(unlocked.Meta.Address)
		if err != nil {
			return fundMsg{err: err}
		}
		if count > 2 {
			count = 2
		}
		return fundMsg{result: nebula.FundResult{Hash: hash, FundedCount: count}}
	}
}

func (m model) toggleNetworkCmd() tea.Cmd {
	store := m.store
	return func() tea.Msg {
		networkValue, err := store.ToggleNetwork()
		return networkMsg{network: networkValue, err: err}
	}
}

func (m model) unlockWalletCmd(identifier, passphrase string, activate bool) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		var unlocked nebula.UnlockedWallet
		var err error
		if strings.TrimSpace(identifier) == "" {
			unlocked, err = store.UnlockActiveWallet(passphrase)
		} else {
			unlocked, err = store.UnlockWallet(identifier, passphrase)
		}
		if err != nil {
			return loginMsg{err: err}
		}
		if activate {
			if _, err := store.SwitchActiveWallet(unlocked.Meta.Address); err != nil {
				return loginMsg{err: err}
			}
			meta, metaErr := store.ActiveWallet()
			if metaErr == nil {
				unlocked.Meta = meta
			}
		}
		return loginMsg{unlocked: unlocked}
	}
}

func (m model) createWalletCmd(name, passphrase string) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		meta, err := store.CreateWallet(name, passphrase)
		if err != nil {
			return walletSavedMsg{err: err}
		}
		unlocked, err := store.UnlockWallet(meta.Address, passphrase)
		return walletSavedMsg{unlocked: unlocked, err: err}
	}
}

func (m model) importWalletCmd(name, secret, passphrase string) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		meta, err := store.ImportWallet(name, secret, passphrase)
		if err != nil {
			return walletSavedMsg{err: err}
		}
		unlocked, err := store.UnlockWallet(meta.Address, passphrase)
		return walletSavedMsg{unlocked: unlocked, err: err}
	}
}

func copyAddressCmd(address string) tea.Cmd {
	return func() tea.Msg {
		return clipboardMsg{err: clipboard.WriteAll(address)}
	}
}

func (m *model) focusCreateInput() {
	m.focusIndex = 0
	m.nameInput.SetValue("")
	m.secretInput.SetValue("")
	m.nameInput.Placeholder = "Wallet name"
	m.secretInput.Placeholder = "Wallet passphrase"
	m.nameInput.Focus()
	m.secretInput.Blur()
	m.secretInput.EchoMode = textinput.EchoPassword
	m.secretInput.EchoCharacter = '*'
	m.loginInput.Blur()
}

func (m *model) focusImportInput() {
	m.focusIndex = 0
	m.nameInput.SetValue("")
	m.secretInput.SetValue("")
	m.loginInput.SetValue("")
	m.nameInput.Placeholder = "Wallet name"
	m.secretInput.Placeholder = "Secret seed"
	m.nameInput.Focus()
	m.secretInput.Blur()
	m.secretInput.EchoMode = textinput.EchoNormal
	m.loginInput.Blur()
	m.loginInput.EchoMode = textinput.EchoPassword
	m.loginInput.EchoCharacter = '*'
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
		m.secretInput.Blur()
	} else {
		m.nameInput.Blur()
		m.secretInput.Focus()
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
	m.loginInput.Blur()
	switch m.focusIndex {
	case 0:
		m.nameInput.Focus()
	case 1:
		m.secretInput.Focus()
	case 2:
		m.loginInput.Focus()
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

func (m model) selectedWallet() (nebula.WalletMeta, bool) {
	if len(m.wallets) == 0 || m.walletCursor < 0 || m.walletCursor >= len(m.wallets) {
		return nebula.WalletMeta{}, false
	}
	return m.wallets[m.walletCursor], true
}

func normalizeUIError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, nebula.ErrWalletNotFound):
		return fmt.Errorf("wallet not found. Create or import one first")
	case errors.Is(err, nebula.ErrInvalidPassphrase):
		return fmt.Errorf("invalid passphrase")
	case errors.Is(err, nebula.ErrAccountNotFunded):
		return fmt.Errorf("Account not funded. Use Friendbot on testnet")
	default:
		return err
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

func returnMode(unlocked *nebula.UnlockedWallet) viewMode {
	if unlocked == nil {
		return modeWelcome
	}
	return modeHome
}
