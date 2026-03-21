//go:build windows

package wireguard

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

const (
	pingInterval = 5 * time.Second
	pingTimeout  = 2 * time.Second
)

// PingResult contiene el resultado de un ping a un servidor VPS
type PingResult struct {
	NetworkID string
	Alive     bool
}

// Pinger gestiona goroutines de ping por red activa.
// Cada red conectada recibe su propio goroutine que hace ICMP ping
// al endpoint IP del servidor cada 5 segundos.
type Pinger struct {
	mu      sync.Mutex
	cancels map[string]context.CancelFunc // networkID -> cancel
	results chan PingResult
}

// NewPinger crea un nuevo Pinger listo para usar.
func NewPinger() *Pinger {
	return &Pinger{
		cancels: make(map[string]context.CancelFunc),
		results: make(chan PingResult, 32),
	}
}

// Start arranca el polling de ping para la red dada usando el IP del endpoint.
// endpointIP puede ser "1.2.3.4" o "1.2.3.4:51820" — se stripea el puerto.
func (p *Pinger) Start(networkID, endpointIP string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Si ya hay uno corriendo para esta red, lo cancelamos primero
	if cancel, exists := p.cancels[networkID]; exists {
		cancel()
	}

	ip := stripPort(endpointIP)
	ctx, cancel := context.WithCancel(context.Background())
	p.cancels[networkID] = cancel

	go p.pollLoop(ctx, networkID, ip)
}

// Stop cancela el polling de ping para la red dada.
func (p *Pinger) Stop(networkID string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if cancel, exists := p.cancels[networkID]; exists {
		cancel()
		delete(p.cancels, networkID)
	}
}

// StopAll cancela todos los pollings activos.
func (p *Pinger) StopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, cancel := range p.cancels {
		cancel()
		delete(p.cancels, id)
	}
}

// Results devuelve el canal de resultados de ping.
// El TUI escucha este canal para actualizar los indicadores.
func (p *Pinger) Results() <-chan PingResult {
	return p.results
}

// pollLoop corre en una goroutine — hace ping cada 5s hasta que ctx sea cancelado.
func (p *Pinger) pollLoop(ctx context.Context, networkID, ip string) {
	// Ping inmediato al arrancar
	alive := pingHost(ip)
	select {
	case p.results <- PingResult{NetworkID: networkID, Alive: alive}:
	case <-ctx.Done():
		return
	}

	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			alive := pingHost(ip)
			select {
			case p.results <- PingResult{NetworkID: networkID, Alive: alive}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// pingHost envía un ICMP echo request al host dado.
// Requiere privilegios de administrador (Prexo ya lo enforcea en main.go).
// Devuelve true si el host responde dentro del timeout.
func pingHost(ip string) bool {
	conn, err := icmp.ListenPacket("ip4:icmp", "0.0.0.0")
	if err != nil {
		// Fallback: intentar TCP connect al puerto 80 como indicador de vida
		return tcpProbe(ip)
	}
	defer conn.Close()

	msg := icmp.Message{
		Type: ipv4.ICMPTypeEcho,
		Code: 0,
		Body: &icmp.Echo{
			ID:   1,
			Seq:  1,
			Data: []byte("prexo"),
		},
	}

	msgBytes, err := msg.Marshal(nil)
	if err != nil {
		return false
	}

	dst := &net.IPAddr{IP: net.ParseIP(ip)}
	if dst.IP == nil {
		// Resolver hostname si fuera necesario
		addrs, err := net.LookupHost(ip)
		if err != nil || len(addrs) == 0 {
			return false
		}
		dst.IP = net.ParseIP(addrs[0])
	}

	_ = conn.SetDeadline(time.Now().Add(pingTimeout))

	if _, err := conn.WriteTo(msgBytes, dst); err != nil {
		return false
	}

	reply := make([]byte, 1500)
	n, _, err := conn.ReadFrom(reply)
	if err != nil {
		return false
	}

	parsed, err := icmp.ParseMessage(1, reply[:n])
	if err != nil {
		return false
	}

	return parsed.Type == ipv4.ICMPTypeEchoReply
}

// tcpProbe intenta conectar TCP al puerto 22 (SSH) como fallback
// cuando ICMP raw sockets no están disponibles.
func tcpProbe(ip string) bool {
	conn, err := net.DialTimeout("tcp", ip+":22", pingTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// stripPort extrae solo el host de una cadena "host:port".
// Si no hay puerto, devuelve la cadena tal cual.
func stripPort(endpoint string) string {
	// Manejar IPv6 entre corchetes: [::1]:51820
	if strings.HasPrefix(endpoint, "[") {
		if idx := strings.LastIndex(endpoint, "]"); idx >= 0 {
			return endpoint[1:idx]
		}
	}
	if idx := strings.LastIndex(endpoint, ":"); idx >= 0 {
		return endpoint[:idx]
	}
	return endpoint
}
