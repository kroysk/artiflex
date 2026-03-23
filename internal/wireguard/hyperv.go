//go:build windows

package wireguard

import (
	"fmt"
	"hash/fnv"
	"os/exec"
	"strings"
)

// HyperVSetup crea un Internal Switch en Hyper-V para la red dada,
// habilita IP Forwarding y configura ICS (Internet Connection Sharing)
// para que SOLO las VMs salgan por la interfaz WireGuard.
//
// switchName  — nombre del switch (igual al nombre de la red en Prexo)
// ifaceName   — nombre de la interfaz WireGuard, ej: "wg-prexo-0"
func HyperVSetup(switchName, ifaceName string) error {
	gatewayIP, _ := hyperVGatewayForSwitch(switchName)
	mask := "255.255.255.0"

	s := switchName
	part1 := fmt.Sprintf(`
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

# Esperar extra para que el stack de red inicialice el adaptador completamente
Start-Sleep -Milliseconds 1500
`, s, s, s, s, s, s)

	part2 := fmt.Sprintf(`
# 3. Asignar gateway del switch: verificar si la correcta ya esta, si no reintentar
$targetIP = '%s'
$targetMask = '%s'
$alias = 'vEthernet (%s)'

$assigned = $false
for ($i = 0; $i -lt 5; $i++) {
    # Verificar si la IP correcta ya esta asignada
    $current = Get-NetIPAddress -InterfaceAlias $alias -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -eq $targetIP }
    if ($current) {
        Write-Host "IP $targetIP ya asignada correctamente a $alias"
        $assigned = $true
        break
    }

    # Eliminar IPs existentes (pueden estar en estado de inicializacion)
    $existing = Get-NetIPAddress -InterfaceAlias $alias -AddressFamily IPv4 -ErrorAction SilentlyContinue
    foreach ($e in $existing) {
        netsh interface ipv4 delete address $alias addr=$($e.IPAddress) 2>&1 | Out-Null
    }
    Start-Sleep -Milliseconds 500

    # Intentar asignar con netsh
    netsh interface ipv4 add address $alias address=$targetIP mask=$targetMask 2>&1 | Out-Null
    Start-Sleep -Milliseconds 600

    # Verificar resultado
    $current = Get-NetIPAddress -InterfaceAlias $alias -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.IPAddress -eq $targetIP }
    if ($current) {
        Write-Host "IP $targetIP asignada a $alias (intento $($i+1))"
        $assigned = $true
        break
    }
    Write-Host "Intento $($i+1) fallido, reintentando..."
    Start-Sleep -Milliseconds 500
}

if (-not $assigned) {
    throw "No se pudo asignar la IP $targetIP a $alias despues de 5 intentos"
}
`, gatewayIP, mask, s)

	part3 := fmt.Sprintf(`
# 4. Habilitar IP Forwarding y metrica alta en ambos adaptadores
Set-NetIPInterface -InterfaceAlias 'vEthernet (%s)' -Forwarding Enabled -InterfaceMetric 9000
Set-NetIPInterface -InterfaceAlias '%s' -Forwarding Enabled -InterfaceMetric 9000 -ErrorAction SilentlyContinue
Write-Host "IP Forwarding habilitado, metrica 9000 asignada"

# 5. Configurar ICS: WireGuard compartido hacia vEthernet del switch.
$publicAlias = '%s'
$privateAlias = 'vEthernet (%s)'

$hnet = New-Object -ComObject HNetCfg.HNetShare
$publicConn = $null
$privateConn = $null

foreach ($conn in $hnet.EnumEveryConnection()) {
    $props = $hnet.NetConnectionProps($conn)
    if ($props.Name -eq $publicAlias) { $publicConn = $conn }
    if ($props.Name -eq $privateAlias) { $privateConn = $conn }
}

if (-not $publicConn) { throw "No se encontro interfaz WireGuard para ICS: $publicAlias" }
if (-not $privateConn) { throw "No se encontro interfaz privada para ICS: $privateAlias" }

# Apagar sharing existente para evitar colisiones (ICS solo permite un public sharing activo)
foreach ($conn in $hnet.EnumEveryConnection()) {
    $cfg = $hnet.INetSharingConfigurationForINetConnection($conn)
    if ($cfg.SharingEnabled) {
        try { $cfg.DisableSharing() } catch {}
    }
}

$publicCfg = $hnet.INetSharingConfigurationForINetConnection($publicConn)
$privateCfg = $hnet.INetSharingConfigurationForINetConnection($privateConn)

# 0 = Public (internet), 1 = Private (home network)
$publicCfg.EnableSharing(0)
$privateCfg.EnableSharing(1)
Write-Host "ICS habilitado: $publicAlias -> $privateAlias"

Write-Host "OK: Hyper-V setup completo para '%s'"
`, s, ifaceName, ifaceName, s, s)

	if err := runPowerShell(part1 + part2 + part3); err != nil {
		return err
	}

	// 6. Asegurar que no quede activo el DHCP embebido de implementaciones previas.
	stopHyperVDHCPForSwitch(switchName)

	return nil
}

// HyperVGatewayIP devuelve la gateway IPv4 que deben usar las VMs para un switch.
func HyperVGatewayIP(switchName string) string {
	_ = switchName
	return "192.168.137.1"
}

// HyperVGatewayCIDR devuelve la subred interna (CIDR) asignada al switch.
func HyperVGatewayCIDR(switchName string) string {
	_ = switchName
	return "192.168.137.0/24"
}

// HyperVSuggestedVMIP devuelve una IP sugerida para una VM dentro de la subred
// interna del switch (reservando .1 para el gateway del host).
func HyperVSuggestedVMIP(switchName string) string {
	_ = switchName
	return "192.168.137.10"
}

func hyperVGatewayForSwitch(switchName string) (gatewayIP string, cidr string) {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(switchName))))
	octet := int(h.Sum32()%254) + 1 // 1..254 dentro de 172.31.0.0/16
	return fmt.Sprintf("172.31.%d.1", octet), fmt.Sprintf("172.31.%d.0/24", octet)
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

# 1. Deshabilitar ICS (si está activo)
$hnet = New-Object -ComObject HNetCfg.HNetShare
foreach ($conn in $hnet.EnumEveryConnection()) {
    $cfg = $hnet.INetSharingConfigurationForINetConnection($conn)
    if ($cfg.SharingEnabled) {
        try { $cfg.DisableSharing() } catch {}
    }
}
Write-Host "ICS deshabilitado"

# 2. Deshabilitar IP Forwarding en el vEthernet y restaurar métrica automática
$adapter = Get-NetAdapter | Where-Object { $_.Name -eq 'vEthernet (%s)' } -ErrorAction SilentlyContinue
if ($adapter) {
    Set-NetIPInterface -InterfaceAlias 'vEthernet (%s)' -Forwarding Disabled -AutomaticMetric Enabled -ErrorAction SilentlyContinue
    Write-Host "IP Forwarding deshabilitado en vEthernet (%s)"
}

# 3. Deshabilitar IP Forwarding en WireGuard y restaurar métrica automática
Set-NetIPInterface -InterfaceAlias '%s' -Forwarding Disabled -AutomaticMetric Enabled -ErrorAction SilentlyContinue
Write-Host "IP Forwarding deshabilitado en %s"

# 4. Eliminar el switch (esto elimina también el adaptador vEthernet)
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

	if err := runPowerShell(script); err != nil {
		return err
	}

	stopHyperVDHCPForSwitch(switchName)
	return nil
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
