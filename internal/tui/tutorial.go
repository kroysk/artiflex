package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Provider identifica el proveedor cloud del tutorial
type Provider int

const (
	ProviderOracle Provider = iota
	ProviderGoogle
)

// tutorialModel es la pantalla de guía paso a paso para configurar el servidor
type tutorialModel struct {
	provider Provider
	viewport viewport.Model
	ready    bool
	width    int
	height   int
}

// newTutorialModel crea el modelo de tutorial para el provider dado.
// width y height son las dimensiones actuales de la terminal — si son > 0
// el viewport se inicializa de inmediato sin esperar WindowSizeMsg.
func newTutorialModel(provider Provider, width, height int) tutorialModel {
	m := tutorialModel{provider: provider, width: width, height: height}
	if width > 0 && height > 0 {
		const headerHeight = 3
		const footerHeight = 2
		m.viewport = viewport.New(width, height-headerHeight-footerHeight)
		m.viewport.SetContent(m.buildContent())
		m.ready = true
	}
	return m
}

// Init implementa tea.Model
func (m tutorialModel) Init() tea.Cmd { return nil }

// Update implementa tea.Model
func (m tutorialModel) Update(msg tea.Msg) (tutorialModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		headerHeight := 3
		footerHeight := 2
		verticalSpace := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalSpace)
			m.viewport.SetContent(m.buildContent())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalSpace
		}
	}

	var cmd tea.Cmd
	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

// View implementa tea.Model
func (m tutorialModel) View() string {
	if !m.ready {
		return "\n  Cargando..."
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		Width(m.width)

	footerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		Padding(0, 1)

	var title string
	switch m.provider {
	case ProviderOracle:
		title = "Prexo — Setup Oracle Cloud (Always Free)"
	case ProviderGoogle:
		title = "Prexo — Setup Google Cloud (Free Tier)"
	}

	scrollPct := int(m.viewport.ScrollPercent() * 100)
	footer := footerStyle.Render("↑/↓ o PgUp/PgDn: scroll  |  esc: volver  |  n: nueva red cuando termines") +
		lipgloss.NewStyle().Foreground(lipgloss.Color("#444")).
			Render("  "+strings.Repeat("─", m.width-60)+" "+lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")).Render(fmt.Sprintf("%d%%", scrollPct)))

	return titleStyle.Render(title) + "\n" +
		m.viewport.View() + "\n" +
		footer
}

// buildContent genera el contenido del tutorial según el provider
func (m tutorialModel) buildContent() string {
	switch m.provider {
	case ProviderOracle:
		return buildOracleTutorial()
	case ProviderGoogle:
		return buildGoogleTutorial()
	}
	return ""
}

// ─── Estilos compartidos ──────────────────────────────────────────────────────

var (
	stepStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))

	codeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#50FA7B")).
			Background(lipgloss.Color("#1A1A2E")).
			Padding(0, 1)

	noteStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFB86C")).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#FFB86C")).
			Padding(0, 1)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#50FA7B")).
			Bold(true)

	sectionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF79C6")).
			Bold(true).
			MarginTop(1)
)

func line(s ...string) string { return strings.Join(s, "") + "\n" }

// ─── Tutorial Oracle Cloud ────────────────────────────────────────────────────

func buildOracleTutorial() string {
	var sb strings.Builder

	sb.WriteString(line())
	sb.WriteString(line(sectionStyle.Render("━━━  REQUISITOS PREVIOS  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  • Cuenta en Oracle Cloud (cloud.oracle.com)"))
	sb.WriteString(line("  • Los 30 días de Free Trial activos O cuenta Already Free"))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 1 — CREAR LA VM  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("1.1"), "  Entrá a ", codeStyle.Render("cloud.oracle.com")))
	sb.WriteString(line("  ", stepStyle.Render("1.2"), "  Menú (≡) → Compute → Instances → Create Instance"))
	sb.WriteString(line("  ", stepStyle.Render("1.3"), "  Name: ", codeStyle.Render("prexo-server-1")))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("1.4"), "  En \"Image and shape\" → Change Shape:"))
	sb.WriteString(line("         • Shape series: ", codeStyle.Render("Ampere (ARM)")))
	sb.WriteString(line("         • Shape: ", codeStyle.Render("VM.Standard.A1.Flex")))
	sb.WriteString(line("         • CPUs: ", codeStyle.Render("2"), "  RAM: ", codeStyle.Render("12 GB")))
	sb.WriteString(line())
	sb.WriteString(line("  ", noteStyle.Render("⚠  Si dice 'Out of capacity' → probá AD-2, AD-2 o cambiá de región")))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("1.5"), "  En \"Image\" → Change Image → Ubuntu → ", codeStyle.Render("Ubuntu 22.04")))
	sb.WriteString(line("  ", stepStyle.Render("1.6"), "  En \"Networking\":"))
	sb.WriteString(line("         ✅ Assign a public IPv4 address → ", codeStyle.Render("habilitado")))
	sb.WriteString(line("  ", stepStyle.Render("1.7"), "  En \"Add SSH keys\":"))
	sb.WriteString(line("         • Si tenés clave SSH → subila"))
	sb.WriteString(line("         • Si no → click en ", codeStyle.Render("Save Private Key"), " para descargarla"))
	sb.WriteString(line("  ", stepStyle.Render("1.8"), "  Click en ", codeStyle.Render("Create"), " → esperá ~2 minutos"))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 2 — ABRIR PUERTO WIREGUARD  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  Oracle bloquea todos los puertos por defecto. Hay que abrirlos."))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("2.1"), "  Click en tu VM → en \"Primary VNIC\" click en la Subnet"))
	sb.WriteString(line("  ", stepStyle.Render("2.2"), "  Click en \"Default Security List\" → \"Add Ingress Rules\""))
	sb.WriteString(line("  ", stepStyle.Render("2.3"), "  Completá:"))
	sb.WriteString(line("         • Source CIDR: ", codeStyle.Render("0.0.0.0/0")))
	sb.WriteString(line("         • IP Protocol: ", codeStyle.Render("UDP")))
	sb.WriteString(line("         • Destination Port Range: ", codeStyle.Render("51820")))
	sb.WriteString(line("  ", stepStyle.Render("2.4"), "  Click en ", codeStyle.Render("Add Ingress Rules")))
	sb.WriteString(line())
	sb.WriteString(line("  ", noteStyle.Render("⚠  También abrí el firewall interno de Ubuntu con:")))
	sb.WriteString(line("  ", codeStyle.Render("sudo iptables -I INPUT -p udp --dport 51820 -j ACCEPT")))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 3 — CONECTARTE POR SSH  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  Desde Windows Terminal o PowerShell:"))
	sb.WriteString(line())
	sb.WriteString(line("  ", codeStyle.Render("ssh -i tu-clave.key ubuntu@<IP-PUBLICA-DE-LA-VM>")))
	sb.WriteString(line())
	sb.WriteString(line("  La IP pública la encontrás en Oracle: click en tu VM → \"Public IP address\""))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 4 — INSTALAR WIREGUARD  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  Una vez conectado por SSH, corré el script de Prexo:"))
	sb.WriteString(line())
	sb.WriteString(line("  ", codeStyle.Render("curl -fsSL https://raw.githubusercontent.com/kroysk/artiflex/main/scripts/server-setup.sh | sudo bash")))
	sb.WriteString(line())
	sb.WriteString(line("  O si lo tenés local:"))
	sb.WriteString(line("  ", codeStyle.Render("sudo bash server-setup.sh")))
	sb.WriteString(line())
	sb.WriteString(line("  El script te va a mostrar al final:"))
	sb.WriteString(line())
	sb.WriteString(line("  ", successStyle.Render("  Endpoint del servidor:       140.238.x.x:51820")))
	sb.WriteString(line("  ", successStyle.Render("  Clave pública del servidor:  ABC123...XYZ=")))
	sb.WriteString(line())
	sb.WriteString(line("  ", noteStyle.Render("⚠  Guardá esos datos — los necesitás para el Paso 5")))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 5 — AGREGAR LA RED EN PREXO  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  Volvé a Prexo y presioná ", codeStyle.Render("N"), " → Nueva Red"))
	sb.WriteString(line())
	sb.WriteString(line("  Completá el formulario con los datos del Paso 4:"))
	sb.WriteString(line("         • Nombre: ", codeStyle.Render("Oracle-VM-1")))
	sb.WriteString(line("         • Endpoint: ", codeStyle.Render("<IP>:51820")))
	sb.WriteString(line("         • Clave pública del servidor: ", codeStyle.Render("<la que te mostró el script>")))
	sb.WriteString(line("         • IP del cliente: ", codeStyle.Render("10.0.0.2/24")))
	sb.WriteString(line("         • DNS: ", codeStyle.Render("1.1.1.1")))
	sb.WriteString(line())
	sb.WriteString(line("  Prexo genera automáticamente las claves del cliente."))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 6 — REGISTRAR EL CLIENTE EN EL SERVIDOR  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  Prexo te va a mostrar tu clave pública de cliente al crear la red."))
	sb.WriteString(line("  Registrala en el servidor con:"))
	sb.WriteString(line())
	sb.WriteString(line("  ", codeStyle.Render("sudo wg set wg0 peer <TU-CLIENT-PUBLIC-KEY> allowed-ips 10.0.0.2/32")))
	sb.WriteString(line("  ", codeStyle.Render("sudo wg-quick save wg0")))
	sb.WriteString(line())
	sb.WriteString(line("  ", successStyle.Render("✓ Listo. Presioná Space en Prexo para activar la red.")))
	sb.WriteString(line())
	sb.WriteString(line("  Repetí el proceso desde el Paso 1 para tu segunda VM (usá IP 10.0.0.3/24 para el segundo cliente)"))
	sb.WriteString(line())

	return sb.String()
}

// ─── Tutorial Google Cloud ────────────────────────────────────────────────────

func buildGoogleTutorial() string {
	var sb strings.Builder

	sb.WriteString(line())
	sb.WriteString(line(sectionStyle.Render("━━━  REQUISITOS PREVIOS  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  • Cuenta en Google Cloud (console.cloud.google.com)"))
	sb.WriteString(line("  • Tarjeta de crédito registrada (no cobra si usás Always Free)"))
	sb.WriteString(line("  • ", noteStyle.Render("ℹ  Google da $300 de crédito por 90 días + 1 VM e2-micro gratis para siempre")))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 1 — CREAR EL PROYECTO  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("1.1"), "  Entrá a ", codeStyle.Render("console.cloud.google.com")))
	sb.WriteString(line("  ", stepStyle.Render("1.2"), "  Arriba a la izquierda → click en el selector de proyecto → ", codeStyle.Render("New Project")))
	sb.WriteString(line("  ", stepStyle.Render("1.3"), "  Name: ", codeStyle.Render("prexo"), " → Create"))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 2 — CREAR LA VM  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("2.1"), "  Menú (≡) → Compute Engine → VM instances → ", codeStyle.Render("Create Instance")))
	sb.WriteString(line("  ", stepStyle.Render("2.2"), "  Name: ", codeStyle.Render("prexo-server-1")))
	sb.WriteString(line("  ", stepStyle.Render("2.3"), "  Region: cualquiera (para Always Free gratis: ", codeStyle.Render("us-east1"), ", ", codeStyle.Render("us-west1"), " o ", codeStyle.Render("us-central1"), ")"))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("2.4"), "  Machine configuration:"))
	sb.WriteString(line("         • Series: ", codeStyle.Render("E2")))
	sb.WriteString(line("         • Machine type: ", codeStyle.Render("e2-micro"), " (2 vCPU, 1GB RAM)"))
	sb.WriteString(line())
	sb.WriteString(line("  ", noteStyle.Render("⚠  e2-micro en us-east1/us-west1/us-central1 = GRATIS PARA SIEMPRE")))
	sb.WriteString(line("     Cualquier otra región o machine type puede generar costos."))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("2.5"), "  Boot disk → Change:"))
	sb.WriteString(line("         • OS: ", codeStyle.Render("Ubuntu")))
	sb.WriteString(line("         • Version: ", codeStyle.Render("Ubuntu 22.04 LTS")))
	sb.WriteString(line("         • Size: ", codeStyle.Render("30 GB"), " (el máximo gratis)"))
	sb.WriteString(line("  ", stepStyle.Render("2.6"), "  En \"Firewall\":"))
	sb.WriteString(line("         ✅ Allow HTTP traffic"))
	sb.WriteString(line("         ✅ Allow HTTPS traffic"))
	sb.WriteString(line("  ", stepStyle.Render("2.7"), "  Click en ", codeStyle.Render("Create"), " → esperá ~1 minuto"))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 3 — ABRIR PUERTO WIREGUARD  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("3.1"), "  Menú (≡) → VPC Network → Firewall → ", codeStyle.Render("Create Firewall Rule")))
	sb.WriteString(line("  ", stepStyle.Render("3.2"), "  Completá:"))
	sb.WriteString(line("         • Name: ", codeStyle.Render("allow-wireguard")))
	sb.WriteString(line("         • Direction: ", codeStyle.Render("Ingress")))
	sb.WriteString(line("         • Action: ", codeStyle.Render("Allow")))
	sb.WriteString(line("         • Targets: ", codeStyle.Render("All instances in the network")))
	sb.WriteString(line("         • Source IPv4 ranges: ", codeStyle.Render("0.0.0.0/0")))
	sb.WriteString(line("         • Protocols and ports → UDP: ", codeStyle.Render("51820")))
	sb.WriteString(line("  ", stepStyle.Render("3.3"), "  Click en ", codeStyle.Render("Create")))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 4 — CONECTARTE POR SSH  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  Google Cloud tiene SSH directo desde el browser — no necesitás clave:"))
	sb.WriteString(line())
	sb.WriteString(line("  ", stepStyle.Render("4.1"), "  Compute Engine → VM instances → click en ", codeStyle.Render("SSH"), " al lado de tu VM"))
	sb.WriteString(line("         Se abre una terminal directo en el browser ✓"))
	sb.WriteString(line())
	sb.WriteString(line("  O desde Windows Terminal:"))
	sb.WriteString(line("  ", codeStyle.Render("gcloud compute ssh prexo-server-1 --zone=<tu-zona>")))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 5 — INSTALAR WIREGUARD  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  Una vez en la terminal SSH, corré el script de Prexo:"))
	sb.WriteString(line())
	sb.WriteString(line("  ", codeStyle.Render("curl -fsSL https://raw.githubusercontent.com/kroysk/artiflex/main/scripts/server-setup.sh | sudo bash")))
	sb.WriteString(line())
	sb.WriteString(line("  El script te va a mostrar al final:"))
	sb.WriteString(line())
	sb.WriteString(line("  ", successStyle.Render("  Endpoint del servidor:       34.x.x.x:51820")))
	sb.WriteString(line("  ", successStyle.Render("  Clave pública del servidor:  ABC123...XYZ=")))
	sb.WriteString(line())
	sb.WriteString(line("  ", noteStyle.Render("⚠  Guardá esos datos — los necesitás para el Paso 6")))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 6 — AGREGAR LA RED EN PREXO  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  Volvé a Prexo y presioná ", codeStyle.Render("N"), " → Nueva Red"))
	sb.WriteString(line())
	sb.WriteString(line("  Completá el formulario con los datos del Paso 5:"))
	sb.WriteString(line("         • Nombre: ", codeStyle.Render("Google-VM-1")))
	sb.WriteString(line("         • Endpoint: ", codeStyle.Render("<IP>:51820")))
	sb.WriteString(line("         • Clave pública del servidor: ", codeStyle.Render("<la que te mostró el script>")))
	sb.WriteString(line("         • IP del cliente: ", codeStyle.Render("10.0.0.2/24")))
	sb.WriteString(line("         • DNS: ", codeStyle.Render("1.1.1.1")))
	sb.WriteString(line())
	sb.WriteString(line("  Prexo genera automáticamente las claves del cliente."))
	sb.WriteString(line())

	sb.WriteString(line(sectionStyle.Render("━━━  PASO 7 — REGISTRAR EL CLIENTE EN EL SERVIDOR  ━━━")))
	sb.WriteString(line())
	sb.WriteString(line("  Registrá la clave pública del cliente en el servidor:"))
	sb.WriteString(line())
	sb.WriteString(line("  ", codeStyle.Render("sudo wg set wg0 peer <TU-CLIENT-PUBLIC-KEY> allowed-ips 10.0.0.2/32")))
	sb.WriteString(line("  ", codeStyle.Render("sudo wg-quick save wg0")))
	sb.WriteString(line())
	sb.WriteString(line("  ", successStyle.Render("✓ Listo. Presioná Space en Prexo para activar la red.")))
	sb.WriteString(line())
	sb.WriteString(line("  ", noteStyle.Render("ℹ  Google Free Tier solo da 1 VM e2-micro gratis. Para una segunda VM\n     usá los $300 de crédito o considerá Oracle Cloud para la segunda.")))
	sb.WriteString(line())

	return sb.String()
}
