// Package config loads and validates krtica's YAML configuration files
// (Decision #17). Server and agent have separate schemas; SIGHUP
// hot-reload arrives with the dynamic control plane.
package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Server configures `krtica server` (the molehill).
type Server struct {
	// Listen is the address agents dial, e.g. ":7000".
	Listen string `yaml:"listen"`
	// Token authenticates agents (P8: mandatory, no anonymous tunnels).
	Token string `yaml:"token"`
	// Tunnels maps public listeners to advertised service names.
	Tunnels []Tunnel `yaml:"tunnels"`
}

// Tunnel is one public exposure: a listen address routed to a service
// advertised by a connected agent.
type Tunnel struct {
	Name   string `yaml:"name"`
	Listen string `yaml:"listen"`
}

// Agent configures `krtica agent` (the mole).
type Agent struct {
	// Name identifies this agent in logs and, later, the control API.
	Name string `yaml:"name"`
	// Server is the molehill's agent-listener address, e.g. "vps:7000".
	Server string `yaml:"server"`
	Token  string `yaml:"token"`
	// Services maps advertised names to local dial targets.
	Services []Service `yaml:"services"`
}

// Service is one local target the agent exposes through the tunnel.
type Service struct {
	Name   string `yaml:"name"`
	Target string `yaml:"target"`
}

// LoadServer reads and validates a server config from path.
func LoadServer(path string) (*Server, error) {
	var cfg Server
	if err := load(path, &cfg); err != nil {
		return nil, err
	}
	if cfg.Listen == "" {
		return nil, fmt.Errorf("config %s: listen is required", path)
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("config %s: token is required", path)
	}
	for i, tn := range cfg.Tunnels {
		if tn.Name == "" || tn.Listen == "" {
			return nil, fmt.Errorf("config %s: tunnels[%d] needs name and listen", path, i)
		}
	}
	return &cfg, nil
}

// LoadAgent reads and validates an agent config from path.
func LoadAgent(path string) (*Agent, error) {
	var cfg Agent
	if err := load(path, &cfg); err != nil {
		return nil, err
	}
	if cfg.Server == "" {
		return nil, fmt.Errorf("config %s: server is required", path)
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("config %s: token is required", path)
	}
	if len(cfg.Services) == 0 {
		return nil, fmt.Errorf("config %s: at least one service is required", path)
	}
	for i, svc := range cfg.Services {
		if svc.Name == "" || svc.Target == "" {
			return nil, fmt.Errorf("config %s: services[%d] needs name and target", path, i)
		}
	}
	return &cfg, nil
}

func load(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("config %s: %w", path, err)
	}
	return nil
}
