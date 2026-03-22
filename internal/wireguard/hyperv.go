//go:build windows

package wireguard

import (
	"fmt"
	"net/netip"
	"os/exec"
	"strings"
)

// HyperVSetup crea un Internal Switch en Hyper-V para la red dada,
// asigna la IP del cliente al adaptador vEthernet y habilita IP Forwarding
// entre ese adaptador y la interfaz WireGuard del túnel.
//
// switchName  — nombre del switch (igual al nombre de la red en Prexo)
// clientIP    — IP del cliente con prefijo CIDR, ej: "10.0.0.2/24"
// ifaceName   — nombre de la interfaz WireGuard, ej: "wg-prexo-0"
func HyperVSetup(switchName, clientIP, ifaceName string) error {
	prefix, err := netip.ParsePrefix(clientIP)
	if err != nil {
		return fmt.Errorf("IP del cliente inválida %q: %w", clientIP, err)
	}

	ip := prefix.Addr().String()
	prefixLen := prefix.Bits()

	// Script PowerShell que:
	// 1. Crea el Internal Switch si no existe
	// 2. Asigna la IP al adaptador vEthernet que Hyper-V crea automáticamente
	// 3. Habilita IP Forwarding en ambos adaptadores (vEthernet y WireGuard)
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'

# 1. Crear Internal Switch si no existe
$switch = Get-VMSwitch -Name '%s' -ErrorAction SilentlyContinue
if (-not $switch) {
    New-VMSwitch -Name '%s' -SwitchType Internal | Out-Null
    Write-Host "Switch '%s' creado"
} else {
    Write-Host "Switch '%s' ya existe"
}

# 2. Esperar a que Windows cree el adaptador vEthernet
$adapter = $null
$attempts = 0
while (-not $adapter -and $attempts -lt 20) {
    $adapter = Get-NetAdapter | Where-Object { $_.Name -eq 'vEthernet (%s)' }
    if (-not $adapter) {
        Start-Sleep -Milliseconds 500
        $attempts++
    }
}
if (-not $adapter) {
    throw "No se encontro el adaptador vEthernet (%s) despues de 10 segundos"
}

# 3. Asignar IP al vEthernet
# Verificar si la IP deseada ya esta asignada — si es asi, no hacer nada
$currentIP = Get-NetIPAddress -InterfaceAlias 'vEthernet (%s)' -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -eq '%s' }

if (-not $currentIP) {
    # Eliminar todas las IPs IPv4 existentes usando netsh (mas confiable que Remove-NetIPAddress)
    $existingIPs = Get-NetIPAddress -InterfaceAlias 'vEthernet (%s)' -AddressFamily IPv4 -ErrorAction SilentlyContinue
    foreach ($existingIP in $existingIPs) {
        netsh interface ipv4 delete address "vEthernet (%s)" addr=$($existingIP.IPAddress) | Out-Null
    }
    Start-Sleep -Milliseconds 500

    # Asignar la IP deseada
    New-NetIPAddress -InterfaceAlias 'vEthernet (%s)' -IPAddress '%s' -PrefixLength %d -ErrorAction Stop | Out-Null
    Write-Host "IP %s/%d asignada a vEthernet (%s)"
} else {
    Write-Host "IP %s/%d ya estaba asignada a vEthernet (%s)"
}

# 4. Habilitar IP Forwarding en ambos adaptadores y asignar metrica alta
# para que Windows NO los use como ruta por defecto (tu red normal tiene
# metrica ~25, estas interfaces tendran 9000 — ultima prioridad siempre)
Set-NetIPInterface -InterfaceAlias 'vEthernet (%s)' -Forwarding Enabled -InterfaceMetric 9000
Set-NetIPInterface -InterfaceAlias '%s' -Forwarding Enabled -InterfaceMetric 9000 -ErrorAction SilentlyContinue
Write-Host "IP Forwarding habilitado, metrica 9000 asignada"

Write-Host "OK: Hyper-V setup completo para '%s'"
`,
		switchName, switchName, switchName, switchName, // crear switch
		switchName, switchName, // esperar adaptador
		switchName, ip, // verificar IP actual
		switchName, switchName, // eliminar IPs con netsh
		switchName, ip, prefixLen, // asignar IP nueva
		ip, prefixLen, switchName, // log asignacion
		ip, prefixLen, switchName, // log ya existia
		switchName, ifaceName, // forwarding
		switchName, // log final
	)

	return runPowerShell(script)
}

// HyperVTeardown elimina el Internal Switch de Hyper-V y limpia la configuración
// de IP Forwarding asociada a la red.
//
// switchName — nombre del switch a eliminar
// ifaceName  — nombre de la interfaz WireGuard, ej: "wg-prexo-0"
func HyperVTeardown(switchName, ifaceName string) error {
	script := fmt.Sprintf(`
$ErrorActionPreference = 'Stop'

# 1. Deshabilitar IP Forwarding en el vEthernet y restaurar métrica automática
$adapter = Get-NetAdapter | Where-Object { $_.Name -eq 'vEthernet (%s)' } -ErrorAction SilentlyContinue
if ($adapter) {
    Set-NetIPInterface -InterfaceAlias 'vEthernet (%s)' -Forwarding Disabled -AutomaticMetric Enabled -ErrorAction SilentlyContinue
    Write-Host "IP Forwarding deshabilitado en vEthernet (%s)"
}

# 2. Deshabilitar IP Forwarding en WireGuard y restaurar métrica automática
Set-NetIPInterface -InterfaceAlias '%s' -Forwarding Disabled -AutomaticMetric Enabled -ErrorAction SilentlyContinue
Write-Host "IP Forwarding deshabilitado en %s"

# 3. Eliminar el switch (esto elimina también el adaptador vEthernet)
$switch = Get-VMSwitch -Name '%s' -ErrorAction SilentlyContinue
if ($switch) {
    Remove-VMSwitch -Name '%s' -Force | Out-Null
    Write-Host "Switch '%s' eliminado"
} else {
    Write-Host "Switch '%s' no existe, nada que eliminar"
}

Write-Host "OK: Hyper-V teardown completo para '%s'"
`,
		switchName, switchName, switchName, // forwarding vEthernet
		ifaceName, ifaceName, // forwarding WireGuard
		switchName, switchName, switchName, switchName, // eliminar switch
		switchName, // log final
	)

	return runPowerShell(script)
}

// HyperVSwitchExists reporta si el Internal Switch ya existe en Hyper-V
func HyperVSwitchExists(switchName string) bool {
	script := fmt.Sprintf(`
$switch = Get-VMSwitch -Name '%s' -ErrorAction SilentlyContinue
if ($switch) { exit 0 } else { exit 1 }
`, switchName)

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", script)
	return cmd.Run() == nil
}

// runPowerShell ejecuta un script PowerShell como administrador y devuelve el error si falla
func runPowerShell(script string) error {
	cmd := exec.Command(
		"powershell",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", script,
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		if output != "" {
			return fmt.Errorf("%w\n%s", err, output)
		}
		return err
	}
	return nil
}
