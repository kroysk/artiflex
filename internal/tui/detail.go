package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/kroysk/artiflex/internal/config"
	"github.com/kroysk/artiflex/internal/wireguard"
)

// detailModel es la pantalla de detalles de una red
type detailModel struct {
	network   config.Network
	status    wireguard.TunnelStatus
	lastError string // último error de conexión, si lo hay
	width     int
	height    int
}

// newDetailModel crea la pantalla de detalles para una red
func newDetailModel(network config.Network, status wireguard.TunnelStatus, lastError string, w, h int) detailModel {
	return detailModel{
		network:   network,
		status:    status,
		lastError: lastError,
		width:     w,
		height:    h,
	}
}

// Init implementa tea.Model
func (m detailModel) Init() tea.Cmd { return nil }

// Update implementa tea.Model
func (m detailModel) Update(msg tea.Msg) (detailModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Volver a la lista — app.go maneja este caso
			return m, nil
		}
	}
	return m, nil
}

// View implementa tea.Model
func (m detailModel) View() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Width(m.width)

	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9B7DFF")).
		Bold(true)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FAFAFA"))

	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF7F")).
		Bold(true)

	inactiveStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF4444")).
		Bold(true)

	errorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF5F87"))

	separatorStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#444444"))

	sectionStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#9B7DFF")).
		Bold(true)

	codeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF7F")).
		Background(lipgloss.Color("#1A1A2E")).
		Padding(0, 1)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262"))

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		Padding(0, 1)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(1, 3).
		Width(m.width - 8).
		MarginLeft(2)

	isActive := m.status.ServiceState == "Corriendo"

	var statusStr string
	if isActive {
		statusStr = activeStyle.Render("● Activa")
	} else {
		statusStr = inactiveStyle.Render("● Inactiva")
	}

	// Extraer solo la IP sin el prefijo CIDR para el comando (ej: "10.0.0.2/32" → "10.0.0.2/32")
	clientIP := m.network.ClientIP
	if clientIP == "" {
		clientIP = "10.0.0.2/32"
	}

	sep := separatorStyle.Render(strings.Repeat("─", m.width-16))

	var rows []string

	// ── Info de la red ──────────────────────────────────────────────────────
	rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("Nombre:         "), valueStyle.Render(m.network.Name)))
	rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("Endpoint:       "), valueStyle.Render(m.network.ServerEndpoint)))
	rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("IP del cliente: "), valueStyle.Render(clientIP)))
	rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("Estado:         "), statusStr))

	if m.status.InterfaceName != "" {
		rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("Interfaz:       "), valueStyle.Render(m.status.InterfaceName)))
	}

	rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("Servicio Win:   "), valueStyle.Render(m.status.ServiceState)))

	// ── Error (si hay) ──────────────────────────────────────────────────────
	if m.lastError != "" {
		rows = append(rows, "")
		rows = append(rows, labelStyle.Render("Último error:"))
		errLines := wrapText(m.lastError, m.width-20)
		for _, l := range errLines {
			rows = append(rows, errorStyle.Render("  "+l))
		}
	} else if !isActive {
		rows = append(rows, "")
		rows = append(rows, errorStyle.Render("Sin información de error — intentá activar la red con Space"))
	}

	// ── Sección: setup del servidor ─────────────────────────────────────────
	rows = append(rows, "")
	rows = append(rows, sep)
	rows = append(rows, "")
	rows = append(rows, sectionStyle.Render("▸ Setup del servidor WireGuard"))
	rows = append(rows, "")

	// Clave pública del cliente
	pubKey := m.network.ClientPublicKey
	if pubKey == "" {
		pubKey = "(no disponible)"
	}
	rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("Tu clave pública:"), valueStyle.Render(pubKey)))
	rows = append(rows, "")

	// Comando completo listo para copiar
	rows = append(rows, dimStyle.Render("Pegá este comando en tu servidor para autorizar este cliente:"))
	rows = append(rows, "")
	cmd := fmt.Sprintf("sudo wg set wg0 peer %s allowed-ips %s", pubKey, clientIP)
	cmdLines := wrapText(cmd, m.width-20)
	for _, l := range cmdLines {
		rows = append(rows, codeStyle.Render(l))
	}

	box := boxStyle.Render(strings.Join(rows, "\n"))

	out := titleStyle.Render("Prexo — Detalle de red") + "\n\n"
	out += box + "\n\n"
	out += footerStyle.Render("esc: volver  q: salir")

	return out
}

// wrapText parte un texto en líneas de máximo maxWidth caracteres
func wrapText(text string, maxWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}
	var lines []string
	for len(text) > maxWidth {
		lines = append(lines, text[:maxWidth])
		text = text[maxWidth:]
	}
	if len(text) > 0 {
		lines = append(lines, text)
	}
	return lines
}
