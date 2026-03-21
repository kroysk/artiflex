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
