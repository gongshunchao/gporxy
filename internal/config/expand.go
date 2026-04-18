package config

import "fmt"

func Expand(cfg Config) ([]Entry, error) {
	if err := Validate(cfg); err != nil {
		return nil, err
	}

	entries := make([]Entry, 0, len(cfg.Rules))
	for _, rule := range cfg.Rules {
		listen, _ := ParseEndpointRange(rule.Listen)
		target, _ := ParseEndpointRange(rule.Target)

		for offset := 0; offset < listen.Len(); offset++ {
			targetPort := target.StartPort
			if target.Len() > 1 {
				targetPort = target.StartPort + offset
			}

			entries = append(entries, Entry{
				Name:     rule.Name,
				Protocol: rule.Protocol,
				Listen:   fmt.Sprintf("%s:%d", listen.Host, listen.StartPort+offset),
				Target:   fmt.Sprintf("%s:%d", target.Host, targetPort),
			})
		}
	}

	return entries, nil
}
