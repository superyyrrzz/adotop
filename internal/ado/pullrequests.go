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
		return "Assigned to me"
	case TabCreated:
		return "Created by me"
	case TabReviewRequested:
		return "All reviewing"
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
		if f.Tab == TabAssigned && s.MyVote != 0 {
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

type WorkItemRef struct {
	ID  string
	URL string
}

type FileChange struct {
	Path       string
	ChangeType string
}

type PRDetail struct {
	PRSummary
	DescriptionMD string
	WorkItemRefs  []WorkItemRef
	SourceSha     string
	TargetSha     string
}

type rawPRDetail struct {
	rawPR
	Description           string `json:"description"`
	LastMergeSourceCommit struct {
		CommitID string `json:"commitId"`
	} `json:"lastMergeSourceCommit"`
	LastMergeTargetCommit struct {
		CommitID string `json:"commitId"`
	} `json:"lastMergeTargetCommit"`
	WorkItemRefs []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	} `json:"workItemRefs"`
}

func (c *Client) GetPullRequest(ctx context.Context, repoID string, prID int, myID string) (*PRDetail, error) {
	path := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d?includeWorkItemRefs=true",
		url.PathEscape(repoID), prID)
	var r rawPRDetail
	if err := c.GetJSON(ctx, path, &r); err != nil {
		return nil, err
	}
	d := &PRDetail{
		PRSummary:     toSummary(r.rawPR, myID),
		DescriptionMD: r.Description,
		SourceSha:     r.LastMergeSourceCommit.CommitID,
		TargetSha:     r.LastMergeTargetCommit.CommitID,
	}
	for _, w := range r.WorkItemRefs {
		d.WorkItemRefs = append(d.WorkItemRefs, WorkItemRef{ID: w.ID, URL: w.URL})
	}
	return d, nil
}

type rawIteration struct {
	ID int `json:"id"`
}
type rawIterationsResp struct {
	Value []rawIteration `json:"value"`
}
type rawChangeEntry struct {
	ChangeType string `json:"changeType"`
	Item       struct {
		Path string `json:"path"`
	} `json:"item"`
}
type rawChangesResp struct {
	ChangeEntries []rawChangeEntry `json:"changeEntries"`
}

func (c *Client) GetIterationChanges(ctx context.Context, repoID string, prID int) ([]FileChange, error) {
	itPath := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/iterations",
		url.PathEscape(repoID), prID)
	var its rawIterationsResp
	if err := c.GetJSON(ctx, itPath, &its); err != nil {
		return nil, err
	}
	if len(its.Value) == 0 {
		return nil, nil
	}
	latest := its.Value[len(its.Value)-1].ID
	chPath := fmt.Sprintf("/_apis/git/repositories/%s/pullrequests/%d/iterations/%d/changes",
		url.PathEscape(repoID), prID, latest)
	var ch rawChangesResp
	if err := c.GetJSON(ctx, chPath, &ch); err != nil {
		return nil, err
	}
	out := make([]FileChange, 0, len(ch.ChangeEntries))
	for _, e := range ch.ChangeEntries {
		out = append(out, FileChange{Path: e.Item.Path, ChangeType: e.ChangeType})
	}
	return out, nil
}
