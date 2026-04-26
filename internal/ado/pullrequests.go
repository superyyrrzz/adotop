package ado

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type Tab int

const (
	TabAssigned Tab = iota
	TabCreated
	TabReviewRequested
)

func (t Tab) String() string {
	switch t {
	case TabAssigned:
		return "Assigned"
	case TabCreated:
		return "Created"
	case TabReviewRequested:
		return "Review requested"
	}
	return "?"
}

type ListPRFilter struct {
	Project string
	Tab     Tab
	MyID    string
	Top     int
}

type ReviewerVote struct {
	ID          string
	DisplayName string
	Vote        int
	IsRequired  bool
}

type PRSummary struct {
	ID           int
	Title        string
	Repo         string
	RepoID       string
	SourceBranch string
	TargetBranch string
	CreatedAt    time.Time
	Author       string
	URL          string
	Reviewers    []ReviewerVote
	MyVote       int
	Draft        bool
	MergeStatus  string
}

type rawPR struct {
	PullRequestID int    `json:"pullRequestId"`
	Title         string `json:"title"`
	SourceRefName string `json:"sourceRefName"`
	TargetRefName string `json:"targetRefName"`
	CreationDate  string `json:"creationDate"`
	IsDraft       bool   `json:"isDraft"`
	MergeStatus   string `json:"mergeStatus"`
	CreatedBy     struct {
		DisplayName string `json:"displayName"`
	} `json:"createdBy"`
	Repository struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"repository"`
	Reviewers []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Vote        int    `json:"vote"`
		IsRequired  bool   `json:"isRequired"`
	} `json:"reviewers"`
	Links struct {
		Web struct {
			Href string `json:"href"`
		} `json:"web"`
	} `json:"_links"`
}

type listResponse struct {
	Value []rawPR `json:"value"`
}

func (c *Client) ListPullRequests(ctx context.Context, f ListPRFilter) ([]PRSummary, error) {
	if f.Project == "" {
		return nil, fmt.Errorf("ListPullRequests: project required")
	}
	if f.MyID == "" {
		return nil, fmt.Errorf("ListPullRequests: MyID required")
	}
	top := f.Top
	if top <= 0 {
		top = 100
	}
	q := url.Values{}
	q.Set("searchCriteria.status", "active")
	q.Set("$top", fmt.Sprintf("%d", top))
	switch f.Tab {
	case TabCreated:
		q.Set("searchCriteria.creatorId", f.MyID)
	case TabAssigned, TabReviewRequested:
		q.Set("searchCriteria.reviewerId", f.MyID)
	}
	path := "/" + url.PathEscape(f.Project) + "/_apis/git/pullrequests?" + q.Encode()

	var resp listResponse
	if err := c.GetJSON(ctx, path, &resp); err != nil {
		return nil, err
	}
	out := make([]PRSummary, 0, len(resp.Value))
	for _, r := range resp.Value {
		s := toSummary(r, f.MyID)
		if f.Tab == TabReviewRequested && s.MyVote != 0 {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

func toSummary(r rawPR, myID string) PRSummary {
	t, _ := time.Parse(time.RFC3339, r.CreationDate)
	s := PRSummary{
		ID:           r.PullRequestID,
		Title:        r.Title,
		Repo:         r.Repository.Name,
		RepoID:       r.Repository.ID,
		SourceBranch: strings.TrimPrefix(r.SourceRefName, "refs/heads/"),
		TargetBranch: strings.TrimPrefix(r.TargetRefName, "refs/heads/"),
		CreatedAt:    t,
		Author:       r.CreatedBy.DisplayName,
		URL:          r.Links.Web.Href,
		Draft:        r.IsDraft,
		MergeStatus:  r.MergeStatus,
	}
	for _, rv := range r.Reviewers {
		s.Reviewers = append(s.Reviewers, ReviewerVote{
			ID: rv.ID, DisplayName: rv.DisplayName, Vote: rv.Vote, IsRequired: rv.IsRequired,
		})
		if rv.ID == myID {
			s.MyVote = rv.Vote
		}
	}
	return s
}
