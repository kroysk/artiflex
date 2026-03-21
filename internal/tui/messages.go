package tui

// ShutdownMsg es enviado al TUI cuando el OS envía una señal de cierre.
// El TUI limpia todas las redes activas y termina.
type ShutdownMsg struct{}

// NetworkToggledMsg se envía cuando una red cambia de estado activo/inactivo
type NetworkToggledMsg struct {
	NetworkID string
	Active    bool
	Err       error
}

// NetworkDeletedMsg se envía cuando una red es eliminada del store
type NetworkDeletedMsg struct {
	NetworkID string
	Err       error
}

// NetworkAddedMsg se envía cuando se agrega una red nueva
type NetworkAddedMsg struct {
	Err error
}

// PingResultMsg se envía cuando el Pinger recibe un resultado de ping
type PingResultMsg struct {
	NetworkID string
	Alive     bool
}

// ShowDetailMsg se envía cuando el usuario presiona Enter sobre una red
type ShowDetailMsg struct {
	NetworkID string
	LastError string // último error de conexión conocido por la lista
}

// HyperVResultMsg se envía cuando termina una operación de Hyper-V (setup o teardown)
type HyperVResultMsg struct {
	Setup bool  // true = setup, false = teardown
	Err   error // nil si fue exitoso
}
