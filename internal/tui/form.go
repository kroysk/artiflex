package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/eduramirezh/prexo/internal/config"
	"github.com/eduramirezh/prexo/internal/wireguard"
	"github.com/google/uuid"
)

// Campos del formulario
const (
	fieldName = iota
	fieldEndpoint
	fieldServerPublicKey
	fieldClientIP
	fieldDNS
	fieldCount
)

// formModel es la pantalla para crear una nueva red
type formModel struct {
	inputs    [fieldCount]textinput.Model
	focused   int
	store     *config.Store
	wgManager *wireguard.Manager
	err       string
}

// newFormModel crea el formulario de nueva red con valores por defecto
func newFormModel(store *config.Store, wgManager *wireguard.Manager) formModel {
	m := formModel{
		store:     store,
		wgManager: wgManager,
	}

	// Campo: Nombre
	m.inputs[fieldName] = textinput.New()
	m.inputs[fieldName].Placeholder = "Mi Servidor VPN"
	m.inputs[fieldName].Focus()
	m.inputs[fieldName].CharLimit = 32
	m.inputs[fieldName].Width = 40

	// Campo: Endpoint del servidor
	m.inputs[fieldEndpoint] = textinput.New()
	m.inputs[fieldEndpoint].Placeholder = "185.x.x.x:51820"
	m.inputs[fieldEndpoint].CharLimit = 64
	m.inputs[fieldEndpoint].Width = 40

	// Campo: Clave pública del servidor
	m.inputs[fieldServerPublicKey] = textinput.New()
	m.inputs[fieldServerPublicKey].Placeholder = "base64 de 44 chars (del script de setup)"
	m.inputs[fieldServerPublicKey].CharLimit = 64
	m.inputs[fieldServerPublicKey].Width = 50

	// Campo: IP del cliente en la red VPN
	m.inputs[fieldClientIP] = textinput.New()
	m.inputs[fieldClientIP].Placeholder = "10.0.0.2/24"
	m.inputs[fieldClientIP].CharLimit = 20
	m.inputs[fieldClientIP].Width = 20

	// Campo: DNS
	m.inputs[fieldDNS] = textinput.New()
	m.inputs[fieldDNS].Placeholder = "1.1.1.1"
	m.inputs[fieldDNS].CharLimit = 40
	m.inputs[fieldDNS].Width = 30

	return m
}

// Init implementa tea.Model
func (m formModel) Init() tea.Cmd {
	return textinput.Blink
}

// Update implementa tea.Model
func (m formModel) Update(msg tea.Msg) (formModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			m.focused = (m.focused + 1) % fieldCount
			return m.updateFocus()
		case "shift+tab", "up":
			m.focused = (m.focused - 1 + fieldCount) % fieldCount
			return m.updateFocus()
		case "enter":
			if m.focused == fieldCount-1 {
				return m.handleSubmit()
			}
			m.focused = (m.focused + 1) % fieldCount
			return m.updateFocus()
		case "esc":
			// La app principal detecta que el formulario no genera NetworkAddedMsg
			// y vuelve a la lista. Enviamos un msg vacío para señalar cancelación.
			return m, func() tea.Msg { return NetworkAddedMsg{Err: nil} }
		}
	}

	// Actualizar el input activo
	var cmd tea.Cmd
	m.inputs[m.focused], cmd = m.inputs[m.focused].Update(msg)
	return m, cmd
}

// View implementa tea.Model
func (m formModel) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		MarginBottom(1)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#A0A0A0")).
		Width(28)

	focusedLabelStyle := labelStyle.
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)

	labels := [fieldCount]string{
		"Nombre de la red:",
		"Endpoint del servidor:",
		"Clave pública del servidor:",
		"IP del cliente (CIDR):",
		"DNS:",
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render("Prexo — Nueva Red Virtual"))
	sb.WriteString("\n\n")

	for i := range fieldCount {
		label := labels[i]
		if i == m.focused {
			sb.WriteString(focusedLabelStyle.Render(label))
		} else {
			sb.WriteString(labelStyle.Render(label))
		}
		sb.WriteString("  ")
		sb.WriteString(m.inputs[i].View())
		sb.WriteString("\n")
	}

	if m.err != "" {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5F87")).MarginTop(1)
		sb.WriteString("\n")
		sb.WriteString(errStyle.Render("✗ " + m.err))
	}

	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).MarginTop(1)
	sb.WriteString("\n")
	sb.WriteString(helpStyle.Render("tab/↓: siguiente campo  shift+tab/↑: anterior  enter: confirmar  esc: cancelar"))

	return lipgloss.NewStyle().Padding(1, 2).Render(sb.String())
}

// updateFocus actualiza el estado de focus de los inputs
func (m formModel) updateFocus() (formModel, tea.Cmd) {
	cmds := make([]tea.Cmd, fieldCount)
	for i := range fieldCount {
		if i == m.focused {
			cmds[i] = m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
	return m, tea.Batch(cmds...)
}

// handleSubmit valida y guarda la red nueva
func (m formModel) handleSubmit() (formModel, tea.Cmd) {
	name := strings.TrimSpace(m.inputs[fieldName].Value())
	endpoint := strings.TrimSpace(m.inputs[fieldEndpoint].Value())
	serverPubKey := strings.TrimSpace(m.inputs[fieldServerPublicKey].Value())
	clientIP := strings.TrimSpace(m.inputs[fieldClientIP].Value())
	dns := strings.TrimSpace(m.inputs[fieldDNS].Value())

	// Validaciones básicas
	switch {
	case name == "":
		m.err = "El nombre es obligatorio"
		return m, nil
	case endpoint == "":
		m.err = "El endpoint del servidor es obligatorio (host:puerto)"
		return m, nil
	case serverPubKey == "":
		m.err = "La clave pública del servidor es obligatoria"
		return m, nil
	case clientIP == "":
		m.err = "La IP del cliente es obligatoria (ej: 10.0.0.2/24)"
		return m, nil
	}

	if dns == "" {
		dns = "1.1.1.1"
	}

	store := m.store

	return m, func() tea.Msg {
		// Generar par de claves para este cliente
		keyPair, err := wireguard.GenerateKeyPair()
		if err != nil {
			return NetworkAddedMsg{Err: fmt.Errorf("error generando claves: %w", err)}
		}

		network := config.Network{
			ID:               uuid.NewString(),
			Name:             name,
			ServerEndpoint:   endpoint,
			ServerPublicKey:  serverPubKey,
			ClientPrivateKey: keyPair.PrivateKey,
			ClientPublicKey:  keyPair.PublicKey,
			ClientIP:         clientIP,
			DNS:              dns,
			AutoConnect:      false,
		}

		if err := store.Add(network); err != nil {
			return NetworkAddedMsg{Err: fmt.Errorf("error guardando red: %w", err)}
		}

		return NetworkAddedMsg{Err: nil}
	}
}
