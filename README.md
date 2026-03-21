# Prexo

Prexo es una aplicación de terminal (TUI) para Windows que te permite gestionar múltiples conexiones VPN simultáneas usando WireGuard. Cada red aparece como un adaptador de red real en Windows, compatible con Hyper-V Virtual Switches.

Está pensada para quienes tienen uno o más servidores VPS gratuitos (Oracle Cloud, Google Cloud) y quieren usarlos como salidas VPN desde Windows sin instalar software adicional más allá de WireGuard.

---

## Requisitos

### En Windows (tu máquina)
- Windows 10 / 11 de 64 bits
- [WireGuard para Windows](https://www.wireguard.com/install/) instalado
- Ejecutar Prexo **como Administrador** (necesario para crear interfaces de red)

### En cada servidor VPS
- Ubuntu 22.04 o Debian 12 (recomendado)
- Acceso root por SSH
- Puerto UDP abierto (por defecto `51820`) — en Oracle Cloud y Google Cloud esto se configura en el firewall del panel web

---

## Instalación

### 1. Descargar Prexo

Descargá `prexo.exe` desde la carpeta del proyecto y guardalo donde quieras (por ejemplo `C:\Tools\prexo.exe`).

### 2. Instalar WireGuard

Descargá e instalá WireGuard desde https://www.wireguard.com/install/

No hace falta configurar nada en WireGuard — Prexo lo maneja todo.

### 3. Configurar el servidor VPS

Conectate a tu VPS por SSH y ejecutá el script de setup:

```bash
curl -fsSL https://raw.githubusercontent.com/eduramirezh/prexo/main/scripts/server-setup.sh | sudo bash
```

O copiando el archivo manualmente:

```bash
scp scripts/server-setup.sh usuario@tu-vps:/tmp/
ssh usuario@tu-vps "sudo bash /tmp/server-setup.sh"
```

Al finalizar, el script te va a mostrar dos datos que vas a necesitar en Prexo:

```
Endpoint del servidor:       1.2.3.4:51820
Clave pública del servidor:  abc123...xyz=
```

Guardá esos datos — los necesitás en el paso siguiente.

### 4. Ejecutar Prexo

Hacé clic derecho sobre `prexo.exe` → **Ejecutar como administrador**.

Se abre la interfaz de terminal:

```
Prexo — Redes Virtuales

  ○  Mi VPS Oracle          1.2.3.4:51820        [Inactiva]
  ○  Mi VPS Google          5.6.7.8:51820        [Inactiva]
```

### 5. Agregar una red

Presioná `N` para abrir el formulario y completá los datos:

| Campo | Ejemplo | Descripción |
|---|---|---|
| Nombre | `Oracle Cloud` | Nombre descriptivo de la red |
| Endpoint del servidor | `1.2.3.4:51820` | IP y puerto del VPS (del paso 3) |
| Clave pública del servidor | `abc123...=` | Clave pública del VPS (del paso 3) |
| IP del cliente | `10.0.0.2/24` | Tu IP dentro del túnel |
| DNS | `1.1.1.1` | Servidor DNS (opcional) |

La clave privada del cliente se genera automáticamente. Presioná `Enter` para guardar.

### 6. Agregar el cliente al servidor

Después de guardar la red en Prexo, necesitás registrar tu clave pública en el servidor. Prexo te la muestra en el formulario. En el VPS, ejecutá:

```bash
sudo wg set wg0 peer <TU_CLAVE_PUBLICA> allowed-ips 10.0.0.2/32
```

---

## Uso diario

| Tecla | Acción |
|---|---|
| `Space` | Conectar / desconectar la red seleccionada |
| `N` | Agregar nueva red |
| `D` | Eliminar la red seleccionada |
| `O` | Tutorial de setup para Oracle Cloud |
| `G` | Tutorial de setup para Google Cloud |
| `Q` / `Ctrl+C` | Cerrar Prexo (desconecta todas las redes) |

### Indicador de estado

Cada red muestra un punto de color:

| Indicador | Significado |
|---|---|
| `○` gris | Red inactiva (apagada) |
| `●` gris | Conectando / esperando respuesta |
| `●` verde | Conectada y servidor respondiendo |
| `●` rojo | Conectada pero servidor sin respuesta |

---

## Servidores gratuitos recomendados

### Oracle Cloud Always Free
- 2 instancias ARM (VM.Standard.A1.Flex) con 1 OCPU y 6 GB RAM cada una — gratis para siempre
- Registrarse en https://www.oracle.com/cloud/free/
- Si aparece "Out of capacity" en ARM, probá otra Availability Domain o usá VM.Standard.E2.1.Micro (AMD)
- Recordá abrir el puerto `51820/udp` en **Security Lists** del panel de Oracle

### Google Cloud Free Tier
- 1 instancia `e2-micro` en us-east1, us-west1 o us-central1 — gratis para siempre
- Registrarse en https://cloud.google.com/free
- Recordá abrir el puerto `51820/udp` en **Firewall Rules** de VPC Network

---

## Notas técnicas

- Las redes VPN **solo existen mientras Prexo está abierto**. Al cerrar la app, todas las interfaces de red se destruyen automáticamente.
- Al arrancar, Prexo limpia cualquier interfaz `wg-prexo-*` que haya quedado de sesiones anteriores por si la app cerró de forma inesperada.
- La configuración de redes se guarda en `%APPDATA%\Prexo\networks.json`.
- Las claves privadas se almacenan localmente en ese archivo — no se envían a ningún servidor.

---

## Compilar desde el código fuente

Requiere [Go 1.25+](https://go.dev/dl/) instalado.

```bash
git clone https://github.com/eduramirezh/prexo
cd prexo
GOOS=windows GOARCH=amd64 go build -o prexo.exe ./cmd/prexo
```
