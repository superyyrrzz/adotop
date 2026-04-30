package ado

import (
	"context"
	"fmt"
	"net/url"
	"sort"
)

type StatusCheck struct {
	Context string
	State   string
}

type rawStatus struct {
	State       string `json:"state"`
	UpdatedDate string `json:"updatedDate"`
	Context     struct {
		Name  string `json:"name"`
		Genre string `json:"genre"`
	} `json:"context"`
}

type rawStatusesResp struct {
	Value []rawStatus `json:"value"`
}

// GetStatuses returns one StatusCheck per (genre, name) context, using
// the most recent updatedDate. ADO's /pullrequests/{id}/statuses endpoint
// returns the FULL history of every status post — for an active PR with
// multiple builds and policy retries that's hundreds of entries, most
// of them stale "pending" rows from earlier runs. Without dedup the
// detail header shows ancient pending counts even when every check has
// long since succeeded. We pick the freshest entry per context so the
// display matches what the ADO web UI shows.
func (c *Client) GetStatuses(ctx context.Context, repoID string, prID int) ([]StatusCheck, error) {
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/statuses",
		url.PathEscape(repoID), prID)
	var r rawStatusesResp
	if err := c.GetJSON(ctx, path, &r); err != nil {
		return nil, err
	}
	latest := make(map[string]rawStatus, len(r.Value))
	for _, s := range r.Value {
		key := s.Context.Name
		if s.Context.Genre != "" {
			key = s.Context.Genre + "/" + s.Context.Name
		}
		if prev, ok := latest[key]; !ok || s.UpdatedDate > prev.UpdatedDate {
			latest[key] = s
		}
	}
	out := make([]StatusCheck, 0, len(latest))
	for key, s := range latest {
		out = append(out, StatusCheck{Context: key, State: s.State})
	}
	// Stable order so the header doesn't reshuffle on every refresh.
	sort.Slice(out, func(i, j int) bool { return out[i].Context < out[j].Context })
	return out, nil
}
