package config

import "time"

type Config struct {
	Control Control `yaml:"control"`
	Rules   []Rule  `yaml:"rules"`
}

type Control struct {
	Socket                string        `yaml:"socket"`
	UDPSessionIdleTimeout time.Duration `yaml:"udp_session_idle_timeout"`
}

type Rule struct {
	Name     string `yaml:"name"`
	Protocol string `yaml:"protocol"`
	Listen   string `yaml:"listen"`
	Target   string `yaml:"target"`
}

type EndpointRange struct {
	Host      string
	StartPort int
	EndPort   int
}

func (e EndpointRange) Len() int {
	return e.EndPort - e.StartPort + 1
}

type Entry struct {
	Name     string
	Protocol string
	Listen   string
	Target   string
}
