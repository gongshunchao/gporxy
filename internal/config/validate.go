package config

import (
	"fmt"
	"strings"
)

func Validate(cfg Config) error {
	if strings.TrimSpace(cfg.Control.Socket) == "" {
		return fmt.Errorf("control.socket is required")
	}

	seenNames := make(map[string]struct{}, len(cfg.Rules))
	seenListeners := make(map[string]struct{})

	for _, rule := range cfg.Rules {
		if _, ok := seenNames[rule.Name]; ok {
			return fmt.Errorf("duplicate rule name %q", rule.Name)
		}
		seenNames[rule.Name] = struct{}{}

		if rule.Protocol != "tcp" && rule.Protocol != "udp" {
			return fmt.Errorf("invalid protocol %q", rule.Protocol)
		}

		listen, err := ParseEndpointRange(rule.Listen)
		if err != nil {
			return err
		}

		target, err := ParseEndpointRange(rule.Target)
		if err != nil {
			return err
		}

		if err := validateEndpointRange(listen); err != nil {
			return err
		}

		if err := validateEndpointRange(target); err != nil {
			return err
		}

		if listen.Len() == 1 && target.Len() > 1 {
			return fmt.Errorf("single listen port cannot map to target range")
		}

		if listen.Len() > 1 && target.Len() > 1 && listen.Len() != target.Len() {
			return fmt.Errorf("listen and target range lengths must match")
		}

		for port := listen.StartPort; port <= listen.EndPort; port++ {
			key := fmt.Sprintf("%s|%s|%d", rule.Protocol, listen.Host, port)
			if _, ok := seenListeners[key]; ok {
				return fmt.Errorf("duplicate listen endpoint %s:%d/%s", listen.Host, port, rule.Protocol)
			}
			seenListeners[key] = struct{}{}
		}
	}

	return nil
}

func validateEndpointRange(endpoint EndpointRange) error {
	if strings.TrimSpace(endpoint.Host) == "" {
		return fmt.Errorf("host is required")
	}

	if endpoint.StartPort < 1 || endpoint.EndPort > 65535 {
		return fmt.Errorf("port out of range")
	}

	if endpoint.StartPort > endpoint.EndPort {
		return fmt.Errorf("invalid port range")
	}

	return nil
}
