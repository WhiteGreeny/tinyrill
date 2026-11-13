package auth

import (
	"fmt"
	"github.com/mythologyli/zju-connect/client"
)

// Server messages may contain echoed passwords, tickets or authentication data.
// Preserve the operation and numeric result, never the raw response message.
func gatewayAuthError(operation string, code int) error {
	return fmt.Errorf("%s (code %d): %w", operation, code, client.ErrAuthenticationRejected)
}
