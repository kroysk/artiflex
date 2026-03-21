//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/kroysk/artiflex/internal/config"
	"github.com/kroysk/artiflex/internal/tui"
	"github.com/kroysk/artiflex/internal/wireguard"
)

func main() {
	// Verificar que corre como administrador
	if !isAdmin() {
		fmt.Fprintln(os.Stderr, "Prexo requiere privilegios de Administrador para gestionar interfaces de red.")
		fmt.Fprintln(os.Stderr, "Por favor, ejecutá como Administrador.")
		os.Exit(1)
	}

	// Cargar config
	store, err := config.NewStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cargando configuración: %v\n", err)
		os.Exit(1)
	}

	// Inicializar WireGuard manager
	wgManager := wireguard.NewManager()

	// Limpiar interfaces huérfanas de sesiones anteriores
	if err := wgManager.CleanupOrphans(); err != nil {
		fmt.Fprintf(os.Stderr, "Advertencia: no se pudieron limpiar interfaces huérfanas: %v\n", err)
	}

	// Inicializar Pinger para health checks de los VPS
	pinger := wireguard.NewPinger()
	defer pinger.StopAll()

	// Contexto con señales de cierre
	ctx, cancel := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Iniciar TUI
	app := tui.New(store, wgManager, pinger)
	p := tea.NewProgram(app, tea.WithAltScreen())

	// Goroutine para shutdown graceful al recibir señal del OS
	go func() {
		<-ctx.Done()
		p.Send(tui.ShutdownMsg{})
	}()

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error en TUI: %v\n", err)
		// Aseguramos cleanup aunque el TUI falle
		wgManager.ShutdownAll()
		os.Exit(1)
	}
}

// isAdmin verifica si el proceso tiene privilegios de administrador en Windows
func isAdmin() bool {
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	if err != nil {
		return false
	}
	return true
}
