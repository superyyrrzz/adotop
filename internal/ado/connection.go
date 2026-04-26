package ado

import "context"

// ConnectionData mirrors the subset of /_apis/connectionData we use.
type ConnectionData struct {
	AuthenticatedUser struct {
		ID            string `json:"id"`
		ProviderDisplayName string `json:"providerDisplayName"`
		CustomDisplayName   string `json:"customDisplayName"`
	} `json:"authenticatedUser"`
	AuthorizedUser struct {
		ID                  string `json:"id"`
		ProviderDisplayName string `json:"providerDisplayName"`
	} `json:"authorizedUser"`
	InstanceID string `json:"instanceId"`
}

// DisplayName returns the best available display name for the authenticated user.
func (c ConnectionData) DisplayName() string {
	if c.AuthenticatedUser.CustomDisplayName != "" {
		return c.AuthenticatedUser.CustomDisplayName
	}
	if c.AuthenticatedUser.ProviderDisplayName != "" {
		return c.AuthenticatedUser.ProviderDisplayName
	}
	return c.AuthorizedUser.ProviderDisplayName
}

// GetConnectionData calls /_apis/connectionData on the org base URL.
func (c *Client) GetConnectionData(ctx context.Context) (*ConnectionData, error) {
	var out ConnectionData
	if err := c.GetJSON(ctx, "/_apis/connectionData?api-version=7.1-preview", &out); err != nil {
		return nil, err
	}
	return &out, nil
}
