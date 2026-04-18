package control

type Command string

const (
	CommandReload Command = "reload"
	CommandStatus Command = "status"
	CommandStop   Command = "stop"
)

type Request struct {
	Command    Command `json:"command"`
	ConfigYAML string  `json:"config_yaml,omitempty"`
}

type Status struct {
	SocketPath       string `json:"socket_path"`
	State            string `json:"state"`
	RuleCount        int    `json:"rule_count"`
	TCPListenerCount int    `json:"tcp_listener_count"`
	UDPListenerCount int    `json:"udp_listener_count"`
}

type Response struct {
	OK     bool    `json:"ok"`
	Error  string  `json:"error,omitempty"`
	Status *Status `json:"status,omitempty"`
}
