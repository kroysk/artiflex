package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/kroysk/artiflex/internal/config"
	"github.com/kroysk/artiflex/internal/wireguard"
)

// screen identifica qué pantalla está activa
type screen int

const (
	screenList screen = iota
	screenForm
	screenTutorialsMenu
	screenTutorialOracle
	screenTutorialGoogle
	screenTutorialWin7VM
	screenDetail
)

// App es el modelo raíz de bubbletea — orquesta las pantallas
type App struct {
	current       screen
	list          listModel
	form          formModel
	tutorialsMenu tutorialsMenuModel
	tutorial      tutorialModel
	detail        detailModel
	store         *config.Store
	wgManager     *wireguard.Manager
	pinger        *wireguard.Pinger
	width         int
	height        int
}

// New crea la App principal del TUI
func New(store *config.Store, wgManager *wireguard.Manager, pinger *wireguard.Pinger) App {
	return App{
		current:   screenList,
		list:      newListModel(store, wgManager, pinger),
		store:     store,
		wgManager: wgManager,
		pinger:    pinger,
	}
}

// Init implementa tea.Model
func (a App) Init() tea.Cmd {
	return a.list.Init()
}

// Update implementa tea.Model — enruta mensajes a la pantalla activa
func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	// Guardar dimensiones de la terminal para pasarlas a pantallas nuevas
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

	// Señal de apagado del OS o del usuario (Q)
	case ShutdownMsg:
		a.pinger.StopAll()
		a.wgManager.ShutdownAll()
		return a, tea.Quit

	case tea.KeyMsg:
		// Q y Ctrl+C siempre apagan desde cualquier pantalla
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			a.pinger.StopAll()
			a.wgManager.ShutdownAll()
			return a, tea.Quit
		}

		// ESC: navegación hacia atrás
		if msg.String() == "esc" {
			switch a.current {
			case screenTutorialsMenu:
				a.current = screenList
				return a, nil
			case screenTutorialOracle, screenTutorialGoogle, screenTutorialWin7VM:
				a.current = screenTutorialsMenu
				return a, nil
			case screenDetail:
				a.current = screenList
				return a, nil
			}
		}

		// Teclas desde la lista principal
		if a.current == screenList {
			switch msg.String() {
			case "n":
				a.current = screenForm
				a.form = newFormModel(a.store, a.wgManager)
				return a, a.form.Init()
			case "t":
				a.current = screenTutorialsMenu
				a.tutorialsMenu = newTutorialsMenuModel()
				return a, a.tutorialsMenu.Init()
			}
		}

		// N desde cualquier pantalla de tutorial va directo al formulario
		if (a.current == screenTutorialOracle || a.current == screenTutorialGoogle || a.current == screenTutorialWin7VM) && msg.String() == "n" {
			a.current = screenForm
			a.form = newFormModel(a.store, a.wgManager)
			return a, a.form.Init()
		}

	// Tutorial seleccionado desde el menú
	case TutorialSelectedMsg:
		switch msg.Provider {
		case ProviderOracle:
			a.current = screenTutorialOracle
		case ProviderGoogle:
			a.current = screenTutorialGoogle
		case ProviderWin7VM:
			a.current = screenTutorialWin7VM
		}
		a.tutorial = newTutorialModel(msg.Provider, a.width, a.height)
		return a, a.tutorial.Init()

	// Abrir pantalla de detalles de una red
	case ShowDetailMsg:
		if network, found := a.store.GetByID(msg.NetworkID); found {
			status := a.wgManager.GetStatus(msg.NetworkID)
			a.detail = newDetailModel(network, a.wgManager, status, msg.LastError, a.width, a.height)
			a.current = screenDetail
		}
		return a, nil

	// Red agregada (o cancelación del formulario)
	case NetworkAddedMsg:
		a.current = screenList
		if msg.Err == nil {
			a.list.refreshItems()
		} else {
			a.list.err = msg.Err.Error()
		}
		return a, nil

	// Full tunnel toggled: sincronizar lista y detail (estén en la pantalla que estén)
	case FullTunnelToggledMsg:
		var listCmd, detailCmd tea.Cmd
		a.list, listCmd = a.list.Update(msg)
		a.detail, detailCmd = a.detail.Update(msg)
		return a, tea.Batch(listCmd, detailCmd)
	}

	// Delegar a la pantalla activa
	switch a.current {
	case screenList:
		var cmd tea.Cmd
		a.list, cmd = a.list.Update(msg)
		return a, cmd

	case screenForm:
		var cmd tea.Cmd
		a.form, cmd = a.form.Update(msg)
		return a, cmd

	case screenTutorialsMenu:
		var cmd tea.Cmd
		a.tutorialsMenu, cmd = a.tutorialsMenu.Update(msg)
		return a, cmd

	case screenTutorialOracle, screenTutorialGoogle, screenTutorialWin7VM:
		var cmd tea.Cmd
		a.tutorial, cmd = a.tutorial.Update(msg)
		return a, cmd

	case screenDetail:
		var cmd tea.Cmd
		a.detail, cmd = a.detail.Update(msg)
		return a, cmd
	}
	return a, nil
}

// View implementa tea.Model — renderiza la pantalla activa
func (a App) View() string {
	switch a.current {
	case screenForm:
		return a.form.View()
	case screenTutorialsMenu:
		return a.tutorialsMenu.View()
	case screenTutorialOracle, screenTutorialGoogle, screenTutorialWin7VM:
		return a.tutorial.View()
	case screenDetail:
		return a.detail.View()
	default:
		return a.list.View()
	}
}
