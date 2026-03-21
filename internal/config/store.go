package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Network representa una red virtual configurada
type Network struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	ServerEndpoint   string `json:"server_endpoint"` // host:port
	ServerPublicKey  string `json:"server_public_key"`
	ClientPrivateKey string `json:"client_private_key"`
	ClientPublicKey  string `json:"client_public_key"`
	ClientIP         string `json:"client_ip"` // ej: "10.0.0.2/24"
	DNS              string `json:"dns"`       // ej: "1.1.1.1"
	AutoConnect      bool   `json:"auto_connect"`
}

// storeData es la estructura raíz del JSON
type storeData struct {
	Networks []Network `json:"networks"`
}

// Store gestiona la persistencia de redes en JSON
type Store struct {
	mu       sync.RWMutex
	filePath string
	data     storeData
}

// NewStore crea o carga el store desde %APPDATA%\Prexo\networks.json
func NewStore() (*Store, error) {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		appData = os.TempDir()
	}

	dir := filepath.Join(appData, "Prexo")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("no se pudo crear directorio de config: %w", err)
	}

	s := &Store{
		filePath: filepath.Join(dir, "networks.json"),
		data:     storeData{Networks: []Network{}},
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("error leyendo config: %w", err)
	}

	return s, nil
}

// load lee el archivo JSON al store
func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.data)
}

// save persiste el store al archivo JSON
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("error serializando config: %w", err)
	}
	return os.WriteFile(s.filePath, data, 0600)
}

// GetAll devuelve todas las redes configuradas (copia)
func (s *Store) GetAll() []Network {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Network, len(s.data.Networks))
	copy(result, s.data.Networks)
	return result
}

// GetByID busca una red por su ID
func (s *Store) GetByID(id string) (Network, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, n := range s.data.Networks {
		if n.ID == id {
			return n, true
		}
	}
	return Network{}, false
}

// Add agrega una red nueva y persiste
func (s *Store) Add(n Network) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Verificar ID duplicado
	for _, existing := range s.data.Networks {
		if existing.ID == n.ID {
			return fmt.Errorf("ya existe una red con ID %q", n.ID)
		}
	}

	s.data.Networks = append(s.data.Networks, n)
	return s.save()
}

// Update actualiza una red existente y persiste
func (s *Store) Update(n Network) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, existing := range s.data.Networks {
		if existing.ID == n.ID {
			s.data.Networks[i] = n
			return s.save()
		}
	}
	return fmt.Errorf("red %q no encontrada", n.ID)
}

// Delete elimina una red por ID y persiste
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, n := range s.data.Networks {
		if n.ID == id {
			s.data.Networks = append(s.data.Networks[:i], s.data.Networks[i+1:]...)
			return s.save()
		}
	}
	return fmt.Errorf("red %q no encontrada", id)
}
