package main

import "testing"

func TestParsePRArg(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		want    int
		wantErr bool
	}{
		{"none", nil, 0, false},
		{"empty_slice", []string{}, 0, false},
		{"bare_id", []string{"1145743"}, 1145743, false},
		{"id_with_whitespace", []string{"  1145743  "}, 1145743, false},
		{"dev_azure_url", []string{"https://dev.azure.com/myorg/myproject/_git/myrepo/pullrequest/1145743"}, 1145743, false},
		{"dev_azure_url_with_query", []string{"https://dev.azure.com/o/p/_git/r/pullrequest/42?_a=files"}, 42, false},
		{"github_style_pull", []string{"https://example.com/owner/repo/pull/99"}, 99, false},
		{"too_many", []string{"1", "2"}, 0, true},
		{"garbage", []string{"not-a-pr"}, 0, true},
		{"zero_rejected", []string{"0"}, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parsePRArg(c.args)
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if got != c.want {
				t.Fatalf("got %d, want %d", got, c.want)
			}
		})
	}
}
