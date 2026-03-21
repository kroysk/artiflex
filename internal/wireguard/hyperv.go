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
    throw "No se encontró el adaptador vEthernet (%s) después de 10 segundos"
}

# 3. Asignar IP al vEthernet (eliminar existente primero si hay)
$existing = Get-NetIPAddress -InterfaceAlias 'vEthernet (%s)' -AddressFamily IPv4 -ErrorAction SilentlyContinue
if ($existing) {
    Remove-NetIPAddress -InterfaceAlias 'vEthernet (%s)' -AddressFamily IPv4 -Confirm:$false | Out-Null
}
New-NetIPAddress -InterfaceAlias 'vEthernet (%s)' -IPAddress '%s' -PrefixLength %d | Out-Null
Write-Host "IP %s/%d asignada a vEthernet (%s)"

# 4. Habilitar IP Forwarding en ambos adaptadores
Set-NetIPInterface -InterfaceAlias 'vEthernet (%s)' -Forwarding Enabled
Set-NetIPInterface -InterfaceAlias '%s' -Forwarding Enabled
Write-Host "IP Forwarding habilitado"

Write-Host "OK: Hyper-V setup completo para '%s'"
`,
		switchName, switchName, switchName, switchName, // crear switch
		switchName, switchName, // esperar adaptador
		switchName, switchName, // limpiar IP
		switchName, ip, prefixLen, // asignar IP
		ip, prefixLen, switchName, // log
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

# 1. Deshabilitar IP Forwarding en el vEthernet
$adapter = Get-NetAdapter | Where-Object { $_.Name -eq 'vEthernet (%s)' } -ErrorAction SilentlyContinue
if ($adapter) {
    Set-NetIPInterface -InterfaceAlias 'vEthernet (%s)' -Forwarding Disabled -ErrorAction SilentlyContinue
    Write-Host "IP Forwarding deshabilitado en vEthernet (%s)"
}

# 2. Deshabilitar IP Forwarding en WireGuard
Set-NetIPInterface -InterfaceAlias '%s' -Forwarding Disabled -ErrorAction SilentlyContinue
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
