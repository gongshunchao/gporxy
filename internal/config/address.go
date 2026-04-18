package config

import (
	"fmt"
	"strconv"
	"strings"
)

func ParseEndpointRange(raw string) (EndpointRange, error) {
	host, portPart, ok := strings.Cut(raw, ":")
	if !ok || host == "" || portPart == "" {
		return EndpointRange{}, fmt.Errorf("invalid endpoint %q", raw)
	}

	if strings.Contains(portPart, "-") {
		startText, endText, ok := strings.Cut(portPart, "-")
		if !ok {
			return EndpointRange{}, fmt.Errorf("invalid endpoint %q", raw)
		}

		start, err := strconv.Atoi(startText)
		if err != nil {
			return EndpointRange{}, fmt.Errorf("invalid endpoint %q", raw)
		}

		end, err := strconv.Atoi(endText)
		if err != nil {
			return EndpointRange{}, fmt.Errorf("invalid endpoint %q", raw)
		}

		return EndpointRange{Host: host, StartPort: start, EndPort: end}, nil
	}

	port, err := strconv.Atoi(portPart)
	if err != nil {
		return EndpointRange{}, fmt.Errorf("invalid endpoint %q", raw)
	}

	return EndpointRange{Host: host, StartPort: port, EndPort: port}, nil
}
