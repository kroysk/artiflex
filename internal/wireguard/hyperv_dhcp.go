//go:build windows

package wireguard

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

type hyperVDHCPServer struct {
	switchName string
	serverIP   net.IP
	mask       net.IPMask
	rangeBase  [4]byte
	server     *server4.Server

	mu     sync.Mutex
	leases map[string]net.IP // MAC -> IP
	next   byte              // host octet, starts at 10
}

var (
	hyperVDHCPMu     sync.Mutex
	hyperVDHCPActive *hyperVDHCPServer
)

func startHyperVDHCPForSwitch(switchName, gatewayIP, gatewayCIDR string) error {
	hyperVDHCPMu.Lock()
	defer hyperVDHCPMu.Unlock()

	// Solo un servidor DHCP activo: la librería en Windows usa UDP normal y no
	// permite bind por interfaz física de forma fiable. Esto cubre el caso típico
	// (múltiples VMs en un mismo switch).
	if hyperVDHCPActive != nil {
		if hyperVDHCPActive.switchName == switchName {
			return nil
		}
		_ = hyperVDHCPActive.server.Close()
		hyperVDHCPActive = nil
	}

	serverIP := net.ParseIP(gatewayIP).To4()
	if serverIP == nil {
		return fmt.Errorf("gateway IP inválida para DHCP: %q", gatewayIP)
	}

	_, ipNet, err := net.ParseCIDR(gatewayCIDR)
	if err != nil {
		return fmt.Errorf("gateway CIDR inválido para DHCP: %q: %w", gatewayCIDR, err)
	}
	base := ipNet.IP.To4()
	if base == nil {
		return fmt.Errorf("CIDR no es IPv4: %q", gatewayCIDR)
	}

	d := &hyperVDHCPServer{
		switchName: switchName,
		serverIP:   serverIP,
		mask:       ipNet.Mask,
		rangeBase:  [4]byte{base[0], base[1], base[2], base[3]},
		leases:     make(map[string]net.IP),
		next:       10,
	}

	addr := &net.UDPAddr{IP: net.IPv4zero, Port: 67}
	srv, err := server4.NewServer("", addr, d.handle)
	if err != nil {
		return fmt.Errorf("error iniciando servidor DHCPv4: %w", err)
	}
	d.server = srv
	hyperVDHCPActive = d

	go func(s *server4.Server) {
		_ = s.Serve()
	}(srv)

	return nil
}

func stopHyperVDHCPForSwitch(switchName string) {
	hyperVDHCPMu.Lock()
	defer hyperVDHCPMu.Unlock()

	if hyperVDHCPActive == nil {
		return
	}
	if switchName != "" && hyperVDHCPActive.switchName != switchName {
		return
	}
	_ = hyperVDHCPActive.server.Close()
	hyperVDHCPActive = nil
}

func stopAllHyperVDHCP() {
	stopHyperVDHCPForSwitch("")
}

func (d *hyperVDHCPServer) handle(conn net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) {
	if req == nil || req.OpCode != dhcpv4.OpcodeBootRequest {
		return
	}

	reply, err := dhcpv4.NewReplyFromRequest(req)
	if err != nil {
		return
	}

	leaseIP := d.leaseFor(req.ClientHWAddr.String())
	reply.YourIPAddr = leaseIP
	reply.UpdateOption(dhcpv4.OptServerIdentifier(d.serverIP))
	reply.UpdateOption(dhcpv4.OptSubnetMask(d.mask))
	reply.UpdateOption(dhcpv4.OptRouter(d.serverIP))
	reply.UpdateOption(dhcpv4.OptDNS(net.IPv4(1, 1, 1, 1), net.IPv4(8, 8, 8, 8)))
	reply.UpdateOption(dhcpv4.OptIPAddressLeaseTime(12 * time.Hour))
	reply.UpdateOption(dhcpv4.OptRenewTimeValue(6 * time.Hour))
	reply.UpdateOption(dhcpv4.OptRebindingTimeValue(10 * time.Hour))

	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		reply.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
	case dhcpv4.MessageTypeRequest:
		reply.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
	default:
		return
	}

	_, _ = conn.WriteTo(reply.ToBytes(), peer)
}

func (d *hyperVDHCPServer) leaseFor(mac string) net.IP {
	d.mu.Lock()
	defer d.mu.Unlock()

	if ip, ok := d.leases[mac]; ok {
		return ip
	}

	start := d.next
	for {
		host := d.next
		if host < 10 {
			host = 10
		}
		if host > 200 {
			host = 10
		}

		candidate := net.IPv4(d.rangeBase[0], d.rangeBase[1], d.rangeBase[2], host)
		if !d.isLeased(candidate) && !candidate.Equal(d.serverIP) {
			d.leases[mac] = candidate
			d.next = host + 1
			if d.next > 200 {
				d.next = 10
			}
			return candidate
		}

		d.next = host + 1
		if d.next > 200 {
			d.next = 10
		}
		if d.next == start {
			// Pool agotado; fallback determinístico dentro del rango.
			ip := net.IPv4(d.rangeBase[0], d.rangeBase[1], d.rangeBase[2], 200)
			d.leases[mac] = ip
			return ip
		}
	}
}

func (d *hyperVDHCPServer) isLeased(ip net.IP) bool {
	for _, leased := range d.leases {
		if leased.Equal(ip) {
			return true
		}
	}
	return false
}
