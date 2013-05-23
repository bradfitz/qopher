// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package issue

import (
	"encoding/xml"
	"io"
	"regexp"
	"time"

	"qopher/task"
)

const (
	urlPrefix = "https://code.google.com/p/go/issues/detail?id="
	query     = "https://code.google.com/feeds/issues/p/go/issues/full?label=Priority-Triage&status=New&fields=entry(@gd:etag,id,title,updated,author,link[@rel='edit'])&max-results=1000"
)

type issueTask struct{}

func init() {
	task.RegisterType(issueTask{})
}

var idFromURL = regexp.MustCompile(`go/issues/full/(\d+)$`)

func (issueTask) TaskURL(id string) string    { return urlPrefix + id }
func (issueTask) Type() string                { return "issue" }
func (issueTask) PollInterval() time.Duration { return 5 * time.Minute }

func (issueTask) Poll(env task.Env) ([]*task.PolledTask, error) {
	c := env.HTTPClient()
	res, err := c.Get(query)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	issues, err := ParseIssues(res.Body)
	if err != nil {
		return nil, err
	}
	env.Logf("got %d issues", len(issues))
	tasks := make([]*task.PolledTask, 0, len(issues))
	for _, issue := range issues {
		t := &task.PolledTask{
			Title: issue.Title,
		}
		if m := idFromURL.FindStringSubmatch(issue.ID); m != nil {
			t.ID = m[1]
		} else {
			env.Logf("Bogus ID %q", issue.ID)
			continue
		}
		tasks = append(tasks, t)
	}
	env.Logf("returning %d tasks", len(tasks))
	return tasks, nil
}

type feed struct {
	XMLName xml.Name `xml:"feed"`
	Entry   []*Issue `xml:"entry"`
}

type Issue struct {
	ID    string `xml:"id"`
	Title string `xml:"title"`
}

func ParseIssues(r io.Reader) ([]*Issue, error) {
	var f feed
	err := xml.NewDecoder(r).Decode(&f)
	return f.Entry, err
}
