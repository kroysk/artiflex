#!/usr/bin/env bash
# =============================================================================
# Prexo — Script de setup WireGuard para VPS
# Corre este script UNA SOLA VEZ en cada servidor como root.
# Probado en Ubuntu 22.04 / Debian 12
# =============================================================================
set -euo pipefail

# ─── Configuración ────────────────────────────────────────────────────────────
WG_INTERFACE="wg0"
WG_PORT="${WG_PORT:-51820}"
WG_SERVER_IP="${WG_SERVER_IP:-10.0.0.1/24}"
# ─────────────────────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()    { echo -e "${CYAN}[INFO]${NC} $*"; }
success() { echo -e "${GREEN}[OK]${NC}   $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
error()   { echo -e "${RED}[ERROR]${NC} $*"; exit 1; }

# ─── Verificaciones ───────────────────────────────────────────────────────────
[[ $EUID -ne 0 ]] && error "Este script debe correrse como root (sudo)"

if [[ -f /etc/wireguard/${WG_INTERFACE}.conf ]]; then
    warn "WireGuard ya está configurado en este servidor."
    warn "Para agregar un cliente nuevo, usá la opción 'Agregar cliente' más abajo."
    echo ""
fi

# ─── Detectar interfaz de red pública ─────────────────────────────────────────
PUBLIC_IFACE=$(ip route | grep default | awk '{print $5}' | head -1)
SERVER_IP=$(curl -s4 ifconfig.me || curl -s4 icanhazip.com)

info "Interfaz pública detectada: ${PUBLIC_IFACE}"
info "IP pública del servidor:    ${SERVER_IP}"

# ─── Instalar WireGuard ───────────────────────────────────────────────────────
info "Instalando WireGuard..."
if command -v apt-get &>/dev/null; then
    apt-get update -qq
    apt-get install -y wireguard wireguard-tools iptables curl
elif command -v dnf &>/dev/null; then
    dnf install -y wireguard-tools iptables curl
else
    error "Gestor de paquetes no soportado. Instalá WireGuard manualmente."
fi
success "WireGuard instalado"

# ─── Generar claves del servidor ──────────────────────────────────────────────
KEYS_DIR="/etc/wireguard/keys"
mkdir -p "${KEYS_DIR}"
chmod 700 "${KEYS_DIR}"

if [[ ! -f "${KEYS_DIR}/server.key" ]]; then
    info "Generando claves del servidor..."
    wg genkey | tee "${KEYS_DIR}/server.key" | wg pubkey > "${KEYS_DIR}/server.pub"
    chmod 600 "${KEYS_DIR}/server.key"
    success "Claves generadas"
else
    info "Claves del servidor ya existen, reutilizando..."
fi

SERVER_PRIVATE_KEY=$(cat "${KEYS_DIR}/server.key")
SERVER_PUBLIC_KEY=$(cat "${KEYS_DIR}/server.pub")

# ─── Configurar IP forwarding ─────────────────────────────────────────────────
info "Habilitando IP forwarding..."
echo 'net.ipv4.ip_forward=1'       >> /etc/sysctl.conf
echo 'net.ipv6.conf.all.forwarding=1' >> /etc/sysctl.conf
sysctl -p > /dev/null
success "IP forwarding habilitado"

# ─── Crear configuración WireGuard ───────────────────────────────────────────
info "Creando ${WG_INTERFACE}.conf..."
cat > "/etc/wireguard/${WG_INTERFACE}.conf" <<EOF
[Interface]
PrivateKey = ${SERVER_PRIVATE_KEY}
Address = ${WG_SERVER_IP}
ListenPort = ${WG_PORT}
SaveConfig = true

# Reglas de NAT — el tráfico de clientes sale por la interfaz pública
PostUp   = iptables -t nat -A POSTROUTING -o ${PUBLIC_IFACE} -j MASQUERADE
PostDown = iptables -t nat -D POSTROUTING -o ${PUBLIC_IFACE} -j MASQUERADE
PostUp   = iptables -A FORWARD -i ${WG_INTERFACE} -j ACCEPT
PostDown = iptables -D FORWARD -i ${WG_INTERFACE} -j ACCEPT

# Los clientes (peers) se agregan acá via 'wg set' o editando este archivo
EOF

chmod 600 "/etc/wireguard/${WG_INTERFACE}.conf"
success "Configuración creada"

# ─── Habilitar y arrancar WireGuard ──────────────────────────────────────────
info "Habilitando servicio WireGuard..."
systemctl enable "wg-quick@${WG_INTERFACE}"
systemctl start  "wg-quick@${WG_INTERFACE}"
success "WireGuard corriendo"

# ─── Configurar firewall (ufw si está disponible) ────────────────────────────
if command -v ufw &>/dev/null; then
    info "Configurando UFW..."
    ufw allow "${WG_PORT}/udp" comment "WireGuard Prexo"
    success "Puerto ${WG_PORT}/udp abierto en UFW"
else
    info "UFW no detectado. Asegurate de abrir el puerto ${WG_PORT}/udp en tu firewall."
fi

# ─── Output final ─────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║         PREXO — Setup completado correctamente       ║${NC}"
echo -e "${GREEN}╚══════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "  Copiá estos datos en Prexo al crear una nueva red:\n"
echo -e "  ${CYAN}Endpoint del servidor:${NC}       ${SERVER_IP}:${WG_PORT}"
echo -e "  ${CYAN}Clave pública del servidor:${NC}  ${SERVER_PUBLIC_KEY}"
echo ""
echo -e "  ${YELLOW}IP sugerida para el cliente:${NC} 10.0.0.2/24"
echo -e "  ${YELLOW}(si agregás otro cliente:)${NC}  10.0.0.3/24, 10.0.0.4/24, etc."
echo ""
echo -e "  Después de agregar el cliente desde Prexo, agregá su clave pública acá:"
echo -e "  ${CYAN}wg set ${WG_INTERFACE} peer <CLIENT_PUBLIC_KEY> allowed-ips 10.0.0.2/32${NC}"
echo ""
