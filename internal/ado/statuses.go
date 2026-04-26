package ado

import (
	"context"
	"fmt"
	"net/url"
)

type StatusCheck struct {
	Context string
	State   string
}

type rawStatus struct {
	State   string `json:"state"`
	Context struct {
		Name  string `json:"name"`
		Genre string `json:"genre"`
	} `json:"context"`
}

type rawStatusesResp struct {
	Value []rawStatus `json:"value"`
}

func (c *Client) GetStatuses(ctx context.Context, repoID string, prID int) ([]StatusCheck, error) {
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/statuses",
		url.PathEscape(repoID), prID)
	var r rawStatusesResp
	if err := c.GetJSON(ctx, path, &r); err != nil {
		return nil, err
	}
	out := make([]StatusCheck, 0, len(r.Value))
	for _, s := range r.Value {
		ctxName := s.Context.Name
		if s.Context.Genre != "" {
			ctxName = s.Context.Genre + "/" + s.Context.Name
		}
		out = append(out, StatusCheck{Context: ctxName, State: s.State})
	}
	return out, nil
}
