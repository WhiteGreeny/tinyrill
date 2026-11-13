// Package identity defines the protected binding used by secrets and sessions.
package identity

import "github.com/mythologyli/zju-connect/configs"

type Binding struct {
	Namespace string `json:"namespace"`
	Customer  string `json:"customer"`
	Protocol  string `json:"protocol"`
	Server    string `json:"server"`
	Domain    string `json:"domain"`
	Username  string `json:"username"`
	Ref       string `json:"ref"`
}

func ForProfile(p configs.Profile) Binding {
	user := p.Auth.Username
	if p.Auth.Method == "sms" {
		user = p.Auth.Phone
	}
	return Binding{Namespace: "tinyrill/v1", Customer: p.ID, Protocol: p.Protocol, Server: p.Server, Domain: p.Auth.Domain, Username: user, Ref: p.Credentials.Ref}
}
