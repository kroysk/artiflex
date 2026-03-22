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
	mask := prefixLenToMask(prefix.Bits())

	// Script PowerShell:
	// 1. Crea el Internal Switch si no existe
	// 2. Espera el adaptador vEthernet
	// 3. Asigna IP solo si la IP correcta no está ya asignada:
	//    - Elimina IPs existentes con netsh (ignorando errores)
	//    - Asigna la IP deseada con netsh (ignorando "already exists")
	//    - Verifica que la IP correcta quedó asignada al final
	// 4. Habilita IP Forwarding en vEthernet y WireGuard
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

# 2. Esperar a que Windows cree el adaptador vEthernet (hasta 10 segundos)
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

# 3. Asignar IP — verificar primero si la IP correcta ya está asignada
$correctIP = Get-NetIPAddress -InterfaceAlias 'vEthernet (%s)' -AddressFamily IPv4 -ErrorAction SilentlyContinue |
    Where-Object { $_.IPAddress -eq '%s' }

if ($correctIP) {
    Write-Host "IP %s ya estaba asignada correctamente a vEthernet (%s)"
} else {
    # Eliminar IPs existentes con netsh (silencioso — puede fallar si no hay ninguna)
    $existingIPs = Get-NetIPAddress -InterfaceAlias 'vEthernet (%s)' -AddressFamily IPv4 -ErrorAction SilentlyContinue
    foreach ($existingIP in $existingIPs) {
        netsh interface ipv4 delete address "vEthernet (%s)" addr=$($existingIP.IPAddress) 2>$null | Out-Null
    }
    Start-Sleep -Milliseconds 800

    # Asignar con netsh — ignorar exit code porque puede reportar "already exists"
    # aunque la asignación haya sido exitosa
    netsh interface ipv4 add address "vEthernet (%s)" address='%s' mask='%s' 2>$null | Out-Null

    # Verificar que la IP quedó asignada correctamente (esta es la fuente de verdad)
    Start-Sleep -Milliseconds 500
    $assigned = Get-NetIPAddress -InterfaceAlias 'vEthernet (%s)' -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -eq '%s' }
    if (-not $assigned) {
        throw "No se pudo asignar la IP %s a vEthernet (%s) — verificar permisos"
    }
    Write-Host "IP %s/%s asignada a vEthernet (%s)"
}

# 4. Habilitar IP Forwarding y metrica alta en ambos adaptadores
Set-NetIPInterface -InterfaceAlias 'vEthernet (%s)' -Forwarding Enabled -InterfaceMetric 9000
Set-NetIPInterface -InterfaceAlias '%s' -Forwarding Enabled -InterfaceMetric 9000 -ErrorAction SilentlyContinue
Write-Host "IP Forwarding habilitado, metrica 9000 asignada"

Write-Host "OK: Hyper-V setup completo para '%s'"
`,
		switchName, switchName, switchName, switchName, // 1. crear switch (4x %s)
		switchName, switchName, // 2. esperar adaptador (2x %s)
		switchName, ip, // 3. verificar IP correcta (switchName, ip)
		ip, switchName, // log ya existia (ip, switchName)
		switchName, switchName, // eliminar IPs existentes (2x switchName)
		switchName, ip, mask, // netsh add (switchName, ip, mask)
		switchName, ip, // verificar asignacion (switchName, ip)
		ip, switchName, // throw si fallo (ip, switchName)
		ip, mask, switchName, // log asignacion exitosa (ip, mask, switchName)
		switchName, ifaceName, // 4. forwarding (2x)
		switchName, // log final
	)

	return runPowerShell(script)
}

// prefixLenToMask convierte un prefijo CIDR a máscara de subred en notación decimal
// ej: 24 → "255.255.255.0", 16 → "255.255.0.0"
func prefixLenToMask(bits int) string {
	var mask [4]byte
	for i := 0; i < bits; i++ {
		mask[i/8] |= 1 << (7 - uint(i%8))
	}
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
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
