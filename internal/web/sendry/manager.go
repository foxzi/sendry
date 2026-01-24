package sendry

import (
	"context"
	"fmt"
	"sync"

	"github.com/foxzi/sendry/internal/web/config"
)

// Manager manages multiple Sendry server clients
type Manager struct {
	clients map[string]*Client
	servers []config.SendryServer
	mu      sync.RWMutex
}

// NewManager creates a new Sendry manager
func NewManager(servers []config.SendryServer) *Manager {
	m := &Manager{
		clients: make(map[string]*Client),
		servers: servers,
	}

	for _, s := range servers {
		m.clients[s.Name] = NewClient(s.BaseURL, s.APIKey)
	}

	return m
}

// GetClient returns a client by server name
func (m *Manager) GetClient(name string) (*Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	client, ok := m.clients[name]
	if !ok {
		return nil, fmt.Errorf("server %q not found", name)
	}
	return client, nil
}

// GetServers returns all configured servers
func (m *Manager) GetServers() []config.SendryServer {
	return m.servers
}

// GetServerByName returns a server config by name
func (m *Manager) GetServerByName(name string) (*config.SendryServer, error) {
	for i := range m.servers {
		if m.servers[i].Name == name {
			return &m.servers[i], nil
		}
	}
	return nil, fmt.Errorf("server %q not found", name)
}

// ServerStatus represents server status with health info
type ServerStatus struct {
	Name      string
	BaseURL   string
	Env       string
	Online    bool
	Version   string
	Uptime    string
	QueueSize int
	Error     string
}

// GetAllStatus returns health status of all servers
func (m *Manager) GetAllStatus(ctx context.Context) []*ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var wg sync.WaitGroup
	results := make([]*ServerStatus, len(m.servers))

	for i, s := range m.servers {
		wg.Add(1)
		go func(idx int, srv config.SendryServer) {
			defer wg.Done()

			status := &ServerStatus{
				Name:    srv.Name,
				BaseURL: srv.BaseURL,
				Env:     srv.Env,
			}

			client := m.clients[srv.Name]
			health, err := client.Health(ctx)
			if err != nil {
				status.Online = false
				status.Error = err.Error()
			} else {
				status.Online = health.Status == "ok"
				status.Version = health.Version
				status.Uptime = health.Uptime
				if health.Queue != nil {
					status.QueueSize = health.Queue.Pending
				}
			}

			results[idx] = status
		}(i, s)
	}

	wg.Wait()
	return results
}

// GetServerStatus returns status of a single server
func (m *Manager) GetServerStatus(ctx context.Context, name string) (*ServerStatus, error) {
	server, err := m.GetServerByName(name)
	if err != nil {
		return nil, err
	}

	client, err := m.GetClient(name)
	if err != nil {
		return nil, err
	}

	status := &ServerStatus{
		Name:    server.Name,
		BaseURL: server.BaseURL,
		Env:     server.Env,
	}

	health, err := client.Health(ctx)
	if err != nil {
		status.Online = false
		status.Error = err.Error()
	} else {
		status.Online = health.Status == "ok"
		status.Version = health.Version
		status.Uptime = health.Uptime
		if health.Queue != nil {
			status.QueueSize = health.Queue.Pending
		}
	}

	return status, nil
}
