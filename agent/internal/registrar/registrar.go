package registrar

import "spaceship/agent/internal/protocol"

type Service struct{}

func (Service) BuildHelloPayload(token string, hostname string, alias string, platform string, arch string) protocol.HelloPayload {
	return protocol.HelloPayload{
		Token:                token,
		Hostname:             hostname,
		Alias:                alias,
		Platform:             platform,
		Arch:                 arch,
		AgentVersion:         "0.1.0",
		DeclaredCapabilities: []string{"exec", "list_dir", "read_file", "write_file"},
	}
}
