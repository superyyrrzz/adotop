package ado

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
)

type rawItemContent struct {
	Content         string `json:"content"`
	ContentMetadata struct {
		Encoding string `json:"encoding"`
	} `json:"contentMetadata"`
}

func (c *Client) getFileAtCommit(ctx context.Context, repoID, path, sha string) ([]byte, error) {
	q := url.Values{}
	q.Set("path", path)
	q.Set("versionDescriptor.version", sha)
	q.Set("versionDescriptor.versionType", "commit")
	q.Set("includeContent", "true")
	q.Set("$format", "json")
	p := fmt.Sprintf("/_apis/git/repositories/%s/items?%s", url.PathEscape(repoID), q.Encode())
	var r rawItemContent
	if err := c.GetJSON(ctx, p, &r); err != nil {
		return nil, err
	}
	if r.ContentMetadata.Encoding == "base64" {
		return base64.StdEncoding.DecodeString(r.Content)
	}
	return []byte(r.Content), nil
}

func (c *Client) GetFileContents(ctx context.Context, repoID, path, sourceSha, targetSha string) (src, tgt []byte, err error) {
	src, err = c.getFileAtCommit(ctx, repoID, path, sourceSha)
	if err != nil {
		if isNotFound(err) {
			src = nil
		} else {
			return nil, nil, err
		}
	}
	tgt, err = c.getFileAtCommit(ctx, repoID, path, targetSha)
	if err != nil {
		if isNotFound(err) {
			tgt = nil
		} else {
			return nil, nil, err
		}
	}
	return src, tgt, nil
}

func isNotFound(err error) bool {
	if e, ok := err.(*APIError); ok {
		return e.Status == 404
	}
	return false
}
