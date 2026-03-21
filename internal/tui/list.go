package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eduramirezh/prexo/internal/config"
	"github.com/eduramirezh/prexo/internal/wireguard"
)

var (
	pingAliveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF7F"))
	pingDeadStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF4444"))
	pingGrayStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
)

// networkItem implementa list.Item para mostrar redes en la lista
type networkItem struct {
	network   config.Network
	active    bool
	pingAlive bool // true = VPS responde ping (solo válido si active == true)
	pingKnown bool // true = ya recibimos al menos un resultado de ping
}

func (i networkItem) Title() string {
	var dot string
	switch {
	case !i.active:
		dot = pingGrayStyle.Render("○")
	case !i.pingKnown:
		dot = pingGrayStyle.Render("●") // conectada pero sin resultado aún
	case i.pingAlive:
		dot = pingAliveStyle.Render("●") // verde: VPS responde
	default:
		dot = pingDeadStyle.Render("●") // rojo: VPS no responde
	}
	return fmt.Sprintf("%s  %s", dot, i.network.Name)
}

func (i networkItem) Description() string {
	endpoint := i.network.ServerEndpoint
	if i.active {
		return fmt.Sprintf("%-30s  [Activa]  %s", endpoint, i.network.ClientIP)
	}
	return fmt.Sprintf("%-30s  [Inactiva]", endpoint)
}

func (i networkItem) FilterValue() string { return i.network.Name }

// keyMap define los atajos de teclado para la lista
type keyMap struct {
	Toggle key.Binding
	New    key.Binding
	Delete key.Binding
	Detail key.Binding
	Quit   key.Binding
}

var keys = keyMap{
	Toggle: key.NewBinding(
		key.WithKeys(" "),
		key.WithHelp("space", "encender/apagar"),
	),
	New: key.NewBinding(
		key.WithKeys("n"),
		key.WithHelp("n", "nueva red"),
	),
	Delete: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "eliminar"),
	),
	Detail: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "detalles"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c"),
		key.WithHelp("q", "salir"),
	),
}

// listModel es el modelo bubbletea para la pantalla de lista de redes
type listModel struct {
	list       list.Model
	store      *config.Store
	wgManager  *wireguard.Manager
	pinger     *wireguard.Pinger
	pingStatus map[string]bool // networkID -> alive (solo redes con resultado conocido)
	err        string
}

// newListModel crea el modelo de lista con todas las redes del store
func newListModel(store *config.Store, wgManager *wireguard.Manager, pinger *wireguard.Pinger) listModel {
	networks := store.GetAll()
	items := make([]list.Item, len(networks))
	for i, n := range networks {
		items[i] = networkItem{
			network: n,
			active:  wgManager.IsActive(n.ID),
		}
	}

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("#7D56F4")).
		BorderLeftForeground(lipgloss.Color("#7D56F4"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.
		Foreground(lipgloss.Color("#9B7DFF")).
		BorderLeftForeground(lipgloss.Color("#7D56F4"))

	l := list.New(items, delegate, 0, 0)
	l.Title = "Prexo — Redes Virtuales"
	l.Styles.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)

	l.AdditionalFullHelpKeys = func() []key.Binding {
		return []key.Binding{keys.Toggle, keys.New, keys.Delete, keys.Detail}
	}

	return listModel{
		list:       l,
		store:      store,
		wgManager:  wgManager,
		pinger:     pinger,
		pingStatus: make(map[string]bool),
	}
}

// Init implementa tea.Model — arranca el listener de ping results
func (m listModel) Init() tea.Cmd {
	return waitForPingResult(m.pinger)
}

// Update implementa tea.Model
func (m listModel) Update(msg tea.Msg) (listModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height-4)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Toggle):
			return m.handleToggle()
		case key.Matches(msg, keys.New):
			// La app principal maneja este caso cambiando a la pantalla de formulario
		case key.Matches(msg, keys.Delete):
			return m.handleDelete()
		}

	case NetworkToggledMsg:
		if msg.Err != nil {
			m.err = fmt.Sprintf("Error: %v", msg.Err)
		} else {
			m.err = ""
			if msg.Active {
				// Red conectada: arrancar ping para ella
				if n, found := m.store.GetByID(msg.NetworkID); found {
					m.pinger.Start(msg.NetworkID, n.ServerEndpoint)
				}
			} else {
				// Red desconectada: parar ping y limpiar estado
				m.pinger.Stop(msg.NetworkID)
				delete(m.pingStatus, msg.NetworkID)
			}
			m.refreshItems()
		}

	case NetworkDeletedMsg:
		if msg.Err != nil {
			m.err = fmt.Sprintf("Error: %v", msg.Err)
		} else {
			m.err = ""
			m.pinger.Stop(msg.NetworkID)
			delete(m.pingStatus, msg.NetworkID)
			m.refreshItems()
		}

	case NetworkAddedMsg:
		if msg.Err != nil {
			m.err = fmt.Sprintf("Error: %v", msg.Err)
		} else {
			m.err = ""
			m.refreshItems()
		}

	case PingResultMsg:
		m.pingStatus[msg.NetworkID] = msg.Alive
		m.refreshItems()
		// Seguir escuchando resultados
		return m, waitForPingResult(m.pinger)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// View implementa tea.Model
func (m listModel) View() string {
	view := m.list.View()

	if m.err != "" {
		errStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF5F87")).
			Padding(0, 1)
		view += "\n" + errStyle.Render(m.err)
	}

	helpStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		Padding(0, 1)
	view += "\n" + helpStyle.Render("space: encender/apagar  n: nueva red  d: eliminar  t: tutoriales  q: salir")

	return view
}

// selectedNetwork devuelve la red seleccionada actualmente, si hay alguna
func (m listModel) selectedNetwork() (config.Network, bool) {
	item, ok := m.list.SelectedItem().(networkItem)
	if !ok {
		return config.Network{}, false
	}
	return item.network, true
}

// handleToggle encender/apaga la red seleccionada en background
func (m listModel) handleToggle() (listModel, tea.Cmd) {
	network, ok := m.selectedNetwork()
	if !ok {
		return m, nil
	}

	isActive := m.wgManager.IsActive(network.ID)

	return m, func() tea.Msg {
		if isActive {
			err := m.wgManager.Disconnect(network.ID)
			return NetworkToggledMsg{NetworkID: network.ID, Active: false, Err: err}
		}
		err := m.wgManager.Connect(network)
		return NetworkToggledMsg{NetworkID: network.ID, Active: err == nil, Err: err}
	}
}

// handleDelete elimina la red seleccionada
func (m listModel) handleDelete() (listModel, tea.Cmd) {
	network, ok := m.selectedNetwork()
	if !ok {
		return m, nil
	}

	return m, func() tea.Msg {
		// Desconectar primero si está activa
		if m.wgManager.IsActive(network.ID) {
			_ = m.wgManager.Disconnect(network.ID)
		}
		err := m.store.Delete(network.ID)
		return NetworkDeletedMsg{NetworkID: network.ID, Err: err}
	}
}

// refreshItems reconstruye los items de la lista desde el store, aplicando estado de ping
func (m *listModel) refreshItems() {
	networks := m.store.GetAll()
	items := make([]list.Item, len(networks))
	for i, n := range networks {
		active := m.wgManager.IsActive(n.ID)
		alive, known := m.pingStatus[n.ID]
		items[i] = networkItem{
			network:   n,
			active:    active,
			pingAlive: alive,
			pingKnown: known && active,
		}
	}
	m.list.SetItems(items)
}

// waitForPingResult devuelve un tea.Cmd que bloquea hasta el próximo PingResult
func waitForPingResult(p *wireguard.Pinger) tea.Cmd {
	return func() tea.Msg {
		result := <-p.Results()
		return PingResultMsg{NetworkID: result.NetworkID, Alive: result.Alive}
	}
}
