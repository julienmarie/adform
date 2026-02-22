package cli

import (
	"fmt"

	"adform/internal/meta"
	"adform/internal/workspace"
)

func metaClientForAccount(root, account string) (*meta.Client, string, error) {
	token, source, err := workspace.ResolveMetaToken(root, account)
	if err != nil {
		return nil, "", err
	}
	if token == "" {
		return nil, "", fmt.Errorf("empty token resolved for account %q", account)
	}
	return meta.FromToken(token), source, nil
}
