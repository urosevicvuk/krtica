package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Server struct {
	AgentListen string `yaml:"agent_listen"`
	Token string `yaml:"token"`
	Routes []Route `yaml:"routes"`
}

type Route struct {
	Name   string `yaml:"name"`
	Listen string `yaml:"listen"`
}

type Agent struct {
	Name string `yaml:"name"`
	Server string `yaml:"server"`
	Token  string `yaml:"token"`
	Services []Service `yaml:"services"`
}

type Service struct {
	Name   string `yaml:"name"`
	Target string `yaml:"target"`
}

func LoadServer(path string) (*Server, error) {
	var cfg Server
	if err := load(path, &cfg); err != nil {
		return nil, err
	}
	if cfg.AgentListen == "" {
		return nil, fmt.Errorf("config %s: agent_listen is required", path)
	}
	if cfg.Token == "" {
		return nil, fmt.Errorf("config %s: token is required", path)
	}
	for i, rt := range cfg.Routes {
		if rt.Name == "" || rt.Listen == "" {
			return nil, fmt.Errorf("config %s: routes[%d] needs name and listen", path, i)
		}
	}
	return &cfg, nil
}

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
