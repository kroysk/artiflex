package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TutorialSelectedMsg se envía cuando el usuario elige un tutorial del menú
type TutorialSelectedMsg struct {
	Provider Provider
}

// tutorialOption representa una opción en el menú de tutoriales
type tutorialOption struct {
	provider    Provider
	title       string
	description string
	icon        string
}

var tutorialOptions = []tutorialOption{
	{
		provider:    ProviderOracle,
		title:       "Oracle Cloud Always Free",
		description: "2 VMs ARM (VM.Standard.A1.Flex) gratis para siempre — ideal para WireGuard",
		icon:        "◆",
	},
	{
		provider:    ProviderGoogle,
		title:       "Google Cloud Free Tier",
		description: "1 VM e2-micro gratis para siempre en us-east1, us-west1 o us-central1",
		icon:        "◆",
	},
}

// tutorialsMenuModel es la pantalla de selección de tutoriales
type tutorialsMenuModel struct {
	selected int
	width    int
	height   int
}

func newTutorialsMenuModel() tutorialsMenuModel {
	return tutorialsMenuModel{selected: 0}
}

// Init implementa tea.Model
func (m tutorialsMenuModel) Init() tea.Cmd { return nil }

// Update implementa tea.Model
func (m tutorialsMenuModel) Update(msg tea.Msg) (tutorialsMenuModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(tutorialOptions)-1 {
				m.selected++
			}
		case "enter", " ":
			return m, func() tea.Msg {
				return TutorialSelectedMsg{Provider: tutorialOptions[m.selected].provider}
			}
		}
	}
	return m, nil
}

// View implementa tea.Model
func (m tutorialsMenuModel) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Width(m.width)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9B7DFF")).
		Padding(1, 2)

	selectedCardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(1, 3).
		Width(m.width - 8).
		MarginLeft(2)

	normalCardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#444444")).
		Padding(1, 3).
		Width(m.width - 8).
		MarginLeft(2)

	selectedTitleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4"))

	normalTitleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA"))

	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262"))

	selectedIconStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#7D56F4"))

	normalIconStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444444"))

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		Padding(0, 1)

	out := titleStyle.Render("Prexo — Tutoriales de configuración") + "\n"
	out += subtitleStyle.Render("Seleccioná el proveedor cloud para ver la guía paso a paso") + "\n"

	for i, opt := range tutorialOptions {
		var card string
		if i == m.selected {
			icon := selectedIconStyle.Render(opt.icon)
			title := selectedTitleStyle.Render(opt.title)
			desc := descStyle.Render(opt.description)
			card = selectedCardStyle.Render(fmt.Sprintf("%s  %s\n   %s", icon, title, desc))
		} else {
			icon := normalIconStyle.Render(opt.icon)
			title := normalTitleStyle.Render(opt.title)
			desc := descStyle.Render(opt.description)
			card = normalCardStyle.Render(fmt.Sprintf("%s  %s\n   %s", icon, title, desc))
		}
		out += card + "\n"
	}

	out += "\n" + footerStyle.Render("↑/↓: navegar  enter: abrir  esc: volver  q: salir")

	return out
}
