package tools

import (
	"strings"
	"testing"
)

func TestBitbucketToolsRejectMissingArguments(t *testing.T) {
	fake := newFakeBamboo(t)
	_, _, bitbucketTools := newTestTools(t, fake)

	cases := []struct {
		tool    string
		args    map[string]interface{}
		wantMsg string
	}{
		{"bitbucket_add_repo", map[string]interface{}{}, "name is required"},
		{"bitbucket_add_repo", map[string]interface{}{"name": "repo"}, "url is required"},
		{"bitbucket_add_repo", map[string]interface{}{"name": "repo", "url": "https://bb/x.git"}, "username is required"},
		{"bitbucket_add_repo", map[string]interface{}{"name": "repo", "url": "https://bb/x.git", "username": "dscully"}, "token is required"},
		{"bitbucket_get_repo", map[string]interface{}{}, "name is required"},
		{"bitbucket_delete_repo", map[string]interface{}{}, "name is required"},
	}

	handlers := Handlers(nil, nil, bitbucketTools)

	for _, tc := range cases {
		t.Run(tc.tool+"/"+tc.wantMsg, func(t *testing.T) {
			res, err := handlers[tc.tool](tc.args)
			assertToolError(t, res, err, tc.wantMsg)
		})
	}
}

func TestBitbucketRepoRoundTrip(t *testing.T) {
	fake := newFakeBamboo(t)
	_, _, bitbucketTools := newTestTools(t, fake)

	addArgs := map[string]interface{}{
		"name":     "checkout",
		"url":      testBitbucket + "/scm/example/checkout.git",
		"username": "dscully",
		"token":    "bitbucket-token-abcd",
	}

	addRes, addErr := bitbucketTools.AddRepo(addArgs)
	if added := assertOK(t, addRes, addErr); !strings.Contains(added, "checkout") {
		t.Errorf("add payload = %q, want it to name the repository", added)
	}

	getRes, getErr := bitbucketTools.GetRepo(map[string]interface{}{"name": "checkout"})
	repo := assertJSON(t, assertOK(t, getRes, getErr))

	if repo["url"] != addArgs["url"] {
		t.Errorf("url = %v, want %v", repo["url"], addArgs["url"])
	}
	if repo["username"] != "dscully" {
		t.Errorf("username = %v, want dscully", repo["username"])
	}

	listRes, listErr := bitbucketTools.ListRepos(map[string]interface{}{})
	repos := assertJSON(t, assertOK(t, listRes, listErr))
	if _, ok := repos["checkout"]; !ok {
		t.Errorf("list is missing the stored repository: %v", repos)
	}

	delRes, delErr := bitbucketTools.DeleteRepo(map[string]interface{}{"name": "checkout"})
	assertOK(t, delRes, delErr)

	goneRes, goneErr := bitbucketTools.GetRepo(map[string]interface{}{"name": "checkout"})
	assertToolError(t, goneRes, goneErr, "Failed to get repository")
}

// TestBitbucketToolsNeverReturnFullTokens is the one that matters for a
// credential store: a stored token must never come back out in full.
func TestBitbucketToolsNeverReturnFullTokens(t *testing.T) {
	fake := newFakeBamboo(t)
	_, _, bitbucketTools := newTestTools(t, fake)

	const secret = "bitbucket-token-abcd"

	addRes, addErr := bitbucketTools.AddRepo(map[string]interface{}{
		"name":     "checkout",
		"url":      testBitbucket + "/scm/example/checkout.git",
		"username": "dscully",
		"token":    secret,
	})
	if added := assertOK(t, addRes, addErr); strings.Contains(added, secret) {
		t.Errorf("add echoed the token back: %s", added)
	}

	getRes, getErr := bitbucketTools.GetRepo(map[string]interface{}{"name": "checkout"})
	payload := assertOK(t, getRes, getErr)
	if strings.Contains(payload, secret) {
		t.Errorf("get returned the full token: %s", payload)
	}

	repo := assertJSON(t, payload)
	if repo["token"] != "***abcd" {
		t.Errorf("token = %v, want it masked to ***abcd", repo["token"])
	}

	listRes, listErr := bitbucketTools.ListRepos(map[string]interface{}{})
	if listed := assertOK(t, listRes, listErr); strings.Contains(listed, secret) {
		t.Errorf("list returned the full token: %s", listed)
	}
}

func TestDeleteRepoReportsUnknownName(t *testing.T) {
	fake := newFakeBamboo(t)
	_, _, bitbucketTools := newTestTools(t, fake)

	res, err := bitbucketTools.DeleteRepo(map[string]interface{}{"name": "never-added"})
	assertToolError(t, res, err, "Failed to delete repository")
}

func TestMaskToken(t *testing.T) {
	cases := []struct {
		name  string
		token string
		want  string
	}{
		{"long token keeps the last four", "bitbucket-token-abcd", "***abcd"},
		{"exactly five characters", "abcde", "***bcde"},
		// Anything four characters or shorter would leak in full, so it is
		// masked entirely.
		{"four characters", "abcd", "***"},
		{"single character", "a", "***"},
		{"empty", "", "***"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := maskToken(tc.token); got != tc.want {
				t.Errorf("maskToken(%q) = %q, want %q", tc.token, got, tc.want)
			}
		})
	}
}
