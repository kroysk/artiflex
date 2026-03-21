//go:build windows

package wireguard

import (
	"encoding/base64"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eduramirezh/prexo/internal/config"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"golang.zx2c4.com/wireguard/windows/conf"
)

// InterfaceState representa el estado de una interfaz WireGuard activa
type InterfaceState struct {
	NetworkID     string
	InterfaceName string // ej: "wg-prexo-0"
	Active        bool
}

// Manager gestiona el ciclo de vida de las interfaces WireGuard en Windows
// usando el Windows Service Control Manager (SCM) y el driver de WireGuard.
type Manager struct {
	mu         sync.RWMutex
	interfaces map[string]*InterfaceState // networkID -> state
	prefix     string
	confDir    string // directorio donde se guardan los .conf temporales
}

// NewManager crea un nuevo WireGuard manager.
// confDir es donde se almacenan los archivos .conf temporales de cada túnel.
func NewManager() *Manager {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = os.TempDir()
	}
	confDir := filepath.Join(appData, "Prexo", "tunnels")
	_ = os.MkdirAll(confDir, 0700)

	return &Manager{
		interfaces: make(map[string]*InterfaceState),
		prefix:     "wg-prexo",
		confDir:    confDir,
	}
}

// Connect crea e instala el servicio WireGuard para la red dada.
// La interfaz de red aparece en Windows y en Hyper-V como adaptador real.
func (m *Manager) Connect(network config.Network) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if state, exists := m.interfaces[network.ID]; exists && state.Active {
		return fmt.Errorf("la red %q ya está activa", network.Name)
	}

	ifaceName := m.nextInterfaceName()

	// 1. Construir la configuración WireGuard
	wgConf, err := buildConf(ifaceName, network)
	if err != nil {
		return fmt.Errorf("error construyendo config WireGuard: %w", err)
	}

	// 2. Guardar el .conf en el store de WireGuard Windows
	if err := wgConf.Save(true); err != nil {
		return fmt.Errorf("error guardando config WireGuard: %w", err)
	}

	// 3. Instalar y arrancar el servicio Windows para este túnel
	if err := installAndStartTunnel(ifaceName); err != nil {
		// Limpiar el .conf si falla
		_ = wgConf.Delete()
		return fmt.Errorf("error iniciando túnel WireGuard: %w", err)
	}

	m.interfaces[network.ID] = &InterfaceState{
		NetworkID:     network.ID,
		InterfaceName: ifaceName,
		Active:        true,
	}

	return nil
}

// Disconnect detiene y desinstala el servicio WireGuard para la red dada.
// La interfaz de red desaparece de Windows y de Hyper-V.
func (m *Manager) Disconnect(networkID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.interfaces[networkID]
	if !exists || !state.Active {
		return nil // ya está desconectada, no es error
	}

	if err := stopAndRemoveTunnel(state.InterfaceName); err != nil {
		return fmt.Errorf("error deteniendo túnel %q: %w", state.InterfaceName, err)
	}

	// Eliminar el .conf del store
	if wgConf, err := conf.LoadFromName(state.InterfaceName); err == nil {
		_ = wgConf.Delete()
	}

	state.Active = false
	delete(m.interfaces, networkID)

	return nil
}

// ShutdownAll desconecta todas las redes activas.
// Llamado al cerrar el TUI o ante señal del OS.
func (m *Manager) ShutdownAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, state := range m.interfaces {
		if state.Active {
			_ = stopAndRemoveTunnel(state.InterfaceName)
			if wgConf, err := conf.LoadFromName(state.InterfaceName); err == nil {
				_ = wgConf.Delete()
			}
		}
		delete(m.interfaces, id)
	}
}

// CleanupOrphans busca y elimina servicios WireGuard con prefijo wg-prexo-*
// que hayan quedado activos de sesiones anteriores crasheadas.
func (m *Manager) CleanupOrphans() error {
	names, err := conf.ListConfigNames()
	if err != nil {
		// Si WireGuard no está instalado todavía, no hay huérfanos
		return nil
	}

	for _, name := range names {
		if strings.HasPrefix(name, m.prefix) {
			_ = stopAndRemoveTunnel(name)
			if wgConf, err := conf.LoadFromName(name); err == nil {
				_ = wgConf.Delete()
			}
		}
	}

	return nil
}

// IsActive reporta si una red está activa en esta sesión
func (m *Manager) IsActive(networkID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.interfaces[networkID]
	return exists && state.Active
}

// GetInterfaceName devuelve el nombre de interfaz Windows para una red activa
func (m *Manager) GetInterfaceName(networkID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.interfaces[networkID]
	if !exists || !state.Active {
		return "", false
	}
	return state.InterfaceName, true
}

// ─── Helpers internos ─────────────────────────────────────────────────────────

// nextInterfaceName genera el próximo nombre disponible (wg-prexo-0, wg-prexo-1, ...)
func (m *Manager) nextInterfaceName() string {
	used := make(map[string]bool)
	for _, state := range m.interfaces {
		used[state.InterfaceName] = true
	}
	for i := range 64 {
		name := fmt.Sprintf("%s-%d", m.prefix, i)
		if !used[name] {
			return name
		}
	}
	return fmt.Sprintf("%s-0", m.prefix)
}

// buildConf construye un conf.Config de WireGuard a partir de una Network de Prexo
func buildConf(ifaceName string, network config.Network) (*conf.Config, error) {
	// Parsear clave privada del cliente
	privKey, err := conf.NewPrivateKeyFromString(network.ClientPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("clave privada inválida: %w", err)
	}

	// Parsear IP del cliente (CIDR)
	clientPrefix, err := netip.ParsePrefix(network.ClientIP)
	if err != nil {
		return nil, fmt.Errorf("IP del cliente inválida %q: %w", network.ClientIP, err)
	}

	// Parsear DNS
	var dnsAddrs []netip.Addr
	if network.DNS != "" {
		for _, d := range strings.Split(network.DNS, ",") {
			d = strings.TrimSpace(d)
			if addr, err := netip.ParseAddr(d); err == nil {
				dnsAddrs = append(dnsAddrs, addr)
			}
		}
	}

	// Parsear clave pública del servidor
	serverPubKey, err := conf.NewPrivateKeyFromString(network.ServerPublicKey)
	if err != nil {
		// Intentar como clave pública directa (base64)
		b, err2 := base64.StdEncoding.DecodeString(network.ServerPublicKey)
		if err2 != nil || len(b) != 32 {
			return nil, fmt.Errorf("clave pública del servidor inválida: %w", err)
		}
		var k conf.Key
		copy(k[:], b)
		serverPubKey = &k
	}

	// Parsear endpoint host:port
	endpointParts := strings.LastIndex(network.ServerEndpoint, ":")
	if endpointParts < 0 {
		return nil, fmt.Errorf("endpoint inválido %q — formato esperado: host:puerto", network.ServerEndpoint)
	}
	host := network.ServerEndpoint[:endpointParts]
	portStr := network.ServerEndpoint[endpointParts+1:]
	var port uint16
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return nil, fmt.Errorf("puerto inválido en endpoint: %w", err)
	}

	// AllowedIPs: todo el tráfico pasa por el túnel
	allIPv4, _ := netip.ParsePrefix("0.0.0.0/0")
	allIPv6, _ := netip.ParsePrefix("::/0")

	wgConf := &conf.Config{
		Name: ifaceName,
		Interface: conf.Interface{
			PrivateKey: *privKey,
			Addresses:  []netip.Prefix{clientPrefix},
			DNS:        dnsAddrs,
		},
		Peers: []conf.Peer{
			{
				PublicKey:           *serverPubKey,
				AllowedIPs:          []netip.Prefix{allIPv4, allIPv6},
				Endpoint:            conf.Endpoint{Host: host, Port: port},
				PersistentKeepalive: 25,
			},
		},
	}

	return wgConf, nil
}

// wireguardExePath busca el ejecutable wireguard.exe en las rutas estándar de instalación
func wireguardExePath() (string, error) {
	candidates := []string{
		`C:\Program Files\WireGuard\wireguard.exe`,
		`C:\Program Files (x86)\WireGuard\wireguard.exe`,
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	// Intentar encontrarlo en PATH
	if p, err := exec.LookPath("wireguard.exe"); err == nil {
		return p, nil
	}
	return "", fmt.Errorf("wireguard.exe no encontrado — instalá WireGuard desde https://www.wireguard.com/install/")
}

// installAndStartTunnel instala el túnel como Windows Service y lo arranca.
// Usa wireguard.exe /installtunnelservice <confPath>
func installAndStartTunnel(ifaceName string) error {
	wgExe, err := wireguardExePath()
	if err != nil {
		return err
	}

	// Obtener la ruta del .conf guardado
	wgConf, err := conf.LoadFromName(ifaceName)
	if err != nil {
		return fmt.Errorf("no se pudo cargar config %q: %w", ifaceName, err)
	}

	confPath, err := wgConf.Path()
	if err != nil {
		return fmt.Errorf("no se pudo obtener ruta del config: %w", err)
	}

	// wireguard.exe /installtunnelservice <confPath>
	cmd := exec.Command(wgExe, "/installtunnelservice", confPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("error instalando servicio: %w\n%s", err, string(out))
	}

	// Esperar a que el servicio esté corriendo (máx 10 segundos)
	svcName, err := conf.ServiceNameOfTunnel(ifaceName)
	if err != nil {
		return fmt.Errorf("error obteniendo nombre de servicio: %w", err)
	}

	return waitForService(svcName, svc.Running, 10*time.Second)
}

// stopAndRemoveTunnel detiene y desinstala el servicio Windows del túnel.
// Usa wireguard.exe /uninstalltunnelservice <ifaceName>
func stopAndRemoveTunnel(ifaceName string) error {
	wgExe, err := wireguardExePath()
	if err != nil {
		// Si wireguard.exe no se encuentra, intentar via SCM directamente
		return stopServiceViaSCM(ifaceName)
	}

	cmd := exec.Command(wgExe, "/uninstalltunnelservice", ifaceName)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Si falla el uninstall, intentar vía SCM
		_ = stopServiceViaSCM(ifaceName)
		return fmt.Errorf("error desinstalando servicio: %w\n%s", err, string(out))
	}

	return nil
}

// stopServiceViaSCM detiene un servicio Windows via el Service Control Manager
func stopServiceViaSCM(ifaceName string) error {
	svcName, err := conf.ServiceNameOfTunnel(ifaceName)
	if err != nil {
		return err
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("error conectando al SCM: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(svcName)
	if err != nil {
		return nil // servicio no existe, no es error
	}
	defer s.Close()

	_, err = s.Control(svc.Stop)
	return err
}

// waitForService espera hasta que un servicio Windows llegue al estado deseado
func waitForService(svcName string, desired svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("error conectando al SCM: %w", err)
	}
	defer m.Disconnect()

	for time.Now().Before(deadline) {
		s, err := m.OpenService(svcName)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		status, err := s.Query()
		s.Close()

		if err == nil && status.State == desired {
			return nil
		}

		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("timeout esperando que el servicio %q llegue al estado %v", svcName, desired)
}
