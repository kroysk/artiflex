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
	network       config.Network
	wgManager     *wireguard.Manager
	status        wireguard.TunnelStatus
	lastError     string // último error de conexión, si lo hay
	hypervExists  bool   // si el Internal Switch de Hyper-V ya existe
	hypervPending bool   // operación Hyper-V en curso
	hypervResult  string // último resultado de operación Hyper-V
	hypervErr     bool   // si el último resultado fue error
	fullTunnel    bool   // true = esta red está en modo full tunnel
	ftErr         string // último error de operación full tunnel
	width         int
	height        int
}

// newDetailModel crea la pantalla de detalles para una red
func newDetailModel(network config.Network, wgManager *wireguard.Manager, status wireguard.TunnelStatus, lastError string, w, h int) detailModel {
	hypervExists := wireguard.HyperVSwitchExists(network.Name)
	return detailModel{
		network:      network,
		wgManager:    wgManager,
		status:       status,
		lastError:    lastError,
		hypervExists: hypervExists,
		fullTunnel:   wgManager.IsFullTunnel(network.ID),
		width:        w,
		height:       h,
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
			// app.go maneja la navegación hacia atrás
			return m, nil

		case "f", "F":
			isActive := m.status.ServiceState == "Corriendo"
			if !isActive {
				return m, nil
			}
			network := m.network
			return m, func() tea.Msg {
				if m.wgManager.IsFullTunnel(network.ID) {
					err := m.wgManager.SetSplitTunnel(network.ID, network.ClientIP)
					return FullTunnelToggledMsg{NetworkID: network.ID, FullTunnel: false, Err: err}
				}
				err := m.wgManager.SetFullTunnel(network.ID, network.ClientIP)
				return FullTunnelToggledMsg{NetworkID: network.ID, FullTunnel: true, Err: err}
			}

		case "h":
			if m.hypervPending || m.hypervExists {
				return m, nil
			}
			m.hypervPending = true
			m.hypervResult = "Configurando Hyper-V..."
			m.hypervErr = false
			network := m.network
			ifaceName := m.status.InterfaceName
			return m, func() tea.Msg {
				err := wireguard.HyperVSetup(network.Name, ifaceName)
				return HyperVResultMsg{Setup: true, Err: err}
			}

		case "x":
			if m.hypervPending || !m.hypervExists {
				return m, nil
			}
			m.hypervPending = true
			m.hypervResult = "Eliminando configuración Hyper-V..."
			m.hypervErr = false
			network := m.network
			ifaceName := m.status.InterfaceName
			return m, func() tea.Msg {
				err := wireguard.HyperVTeardown(network.Name, ifaceName)
				return HyperVResultMsg{Setup: false, Err: err}
			}
		}

	case FullTunnelToggledMsg:
		if msg.NetworkID == m.network.ID {
			if msg.Err != nil {
				m.ftErr = fmt.Sprintf("Error full tunnel: %v", msg.Err)
			} else {
				m.ftErr = ""
				m.fullTunnel = msg.FullTunnel
			}
		}

	case HyperVResultMsg:
		m.hypervPending = false
		if msg.Err != nil {
			m.hypervErr = true
			m.hypervResult = fmt.Sprintf("Error: %v", msg.Err)
		} else {
			m.hypervErr = false
			m.hypervExists = msg.Setup // setup=true → existe, teardown=false → no existe
			if msg.Setup {
				m.hypervResult = "✓ Switch Hyper-V configurado — asigná este switch a tu VM"
			} else {
				m.hypervResult = "✓ Configuración Hyper-V eliminada"
			}
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

	successStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#00FF7F"))

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

	pendingStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFD700"))

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

	clientIP := m.network.ClientIP
	if clientIP == "" {
		clientIP = "10.0.0.2/24"
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

	// ── Full Tunnel ─────────────────────────────────────────────────────────
	if isActive {
		ftStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FFD700")).Bold(true)
		var ftStatusStr string
		if m.fullTunnel {
			ftStatusStr = ftStyle.Render("◉ ACTIVO — todo el tráfico pasa por este túnel")
		} else {
			ftStatusStr = dimStyle.Render("○ split (solo tráfico interno)")
		}
		rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("Full Tunnel:    "), ftStatusStr))
		if m.ftErr != "" {
			rows = append(rows, errorStyle.Render("  "+m.ftErr))
		}
	}

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

	pubKey := m.network.ClientPublicKey
	if pubKey == "" {
		pubKey = "(no disponible)"
	}
	rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("Tu clave pública:"), valueStyle.Render(pubKey)))
	rows = append(rows, "")
	rows = append(rows, dimStyle.Render("Pegá este comando en tu servidor para autorizar este cliente:"))
	rows = append(rows, "")
	cmd := fmt.Sprintf("sudo wg set wg0 peer %s allowed-ips %s", pubKey, clientIP)
	for _, l := range wrapText(cmd, m.width-20) {
		rows = append(rows, codeStyle.Render(l))
	}

	// ── Sección: Hyper-V ────────────────────────────────────────────────────
	rows = append(rows, "")
	rows = append(rows, sep)
	rows = append(rows, "")
	rows = append(rows, sectionStyle.Render("▸ Hyper-V Internal Switch"))
	rows = append(rows, "")

	// Estado del switch
	var hypervStatusStr string
	if m.hypervExists {
		hypervStatusStr = successStyle.Render("● Configurado — switch \"" + m.network.Name + "\" existe")
	} else {
		hypervStatusStr = dimStyle.Render("○ No configurado")
	}
	rows = append(rows, fmt.Sprintf("%s %s", labelStyle.Render("Estado Hyper-V: "), hypervStatusStr))

	// Resultado de la última operación
	if m.hypervPending {
		rows = append(rows, "")
		rows = append(rows, pendingStyle.Render("  ⟳ "+m.hypervResult))
	} else if m.hypervResult != "" {
		rows = append(rows, "")
		if m.hypervErr {
			for _, l := range wrapText(m.hypervResult, m.width-20) {
				rows = append(rows, errorStyle.Render("  "+l))
			}
		} else {
			rows = append(rows, successStyle.Render("  "+m.hypervResult))
		}
	}

	// Instrucciones según estado
	if !m.hypervExists && !m.hypervPending {
		rows = append(rows, "")
		if !isActive {
			rows = append(rows, dimStyle.Render("  Activá la red primero con Space antes de configurar Hyper-V"))
		} else {
			rows = append(rows, dimStyle.Render("  Presioná H para crear el Internal Switch y habilitar routing"))
		}
	} else if m.hypervExists && !m.hypervPending {
		rows = append(rows, "")
		gwIP := wireguard.HyperVGatewayIP(m.network.Name)
		gwCIDR := wireguard.HyperVGatewayCIDR(m.network.Name)
		vmIP := wireguard.HyperVSuggestedVMIP(m.network.Name)
		rows = append(rows, dimStyle.Render("  En Hyper-V: Settings → Network Adapter → Virtual switch: \""+m.network.Name+"\""))
		rows = append(rows, dimStyle.Render("  Gateway en la VM: "+gwIP))
		rows = append(rows, dimStyle.Render("  Subred VM (NAT): "+gwCIDR))
		rows = append(rows, "")
		rows = append(rows, sectionStyle.Render("  Config VM sugerida"))
		rows = append(rows, dimStyle.Render("    IP:      "+vmIP))
		rows = append(rows, dimStyle.Render("    Máscara: 255.255.255.0"))
		rows = append(rows, dimStyle.Render("    Gateway: "+gwIP))
		rows = append(rows, dimStyle.Render("    DNS:     1.1.1.1, 8.8.8.8"))
	}

	box := boxStyle.Render(strings.Join(rows, "\n"))

	// Footer dinámico según estado
	var footerParts []string
	footerParts = append(footerParts, "esc: volver")
	if isActive {
		footerParts = append(footerParts, "f: full tunnel")
	}
	if !m.hypervPending {
		if !m.hypervExists && isActive {
			footerParts = append(footerParts, "h: configurar Hyper-V")
		}
		if m.hypervExists {
			footerParts = append(footerParts, "x: eliminar Hyper-V")
		}
	}
	footerParts = append(footerParts, "q: salir")

	out := titleStyle.Render("Prexo — Detalle de red") + "\n\n"
	out += box + "\n\n"
	out += footerStyle.Render(strings.Join(footerParts, "  "))

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
