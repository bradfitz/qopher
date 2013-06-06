// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package issue

import (
	"encoding/xml"
	"io"
	"regexp"
	"strings"
	"time"

	"qopher/task"
)

const (
	urlPrefix = "https://code.google.com/p/go/issues/detail?id="
	query     = "https://code.google.com/feeds/issues/p/go/issues/full?label=Priority-Triage&status=New&fields=entry(@gd:etag,id,title,published,updated,author,issues:owner,link[@rel='edit'])&max-results=1000"
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
		if issue.Owner != nil {
			// Owned issues aren't logically open
			continue
		}
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

	// TODO:
	// <published>2013-05-14T07:09:06.000Z</published>

	// TODO:
	// <updated>2013-05-05T01:56:19.000Z</updated>

	// <issues:owner><issues:uri>/u/102602228801687104398/</issues:uri><issues:username>g...@golang.org</issues:username></issues:owner>
	Owner *IssuePerson `xml:"http://schemas.google.com/projecthosting/issues/2009 owner"`
}

type IssuePerson struct {
	// Like "/u/102602228801687104398/"
	URI string `xml:"http://schemas.google.com/projecthosting/issues/2009 uri"`

	// Useless username: "g..@golang.org"
	Username string `xml:"http://schemas.google.com/projecthosting/issues/2009 username"`
}

func ParseIssues(r io.Reader) ([]*Issue, error) {
	var f feed
	err := xml.NewDecoder(r).Decode(&f)
	return f.Entry, err
}

// Start-up time sanity check of parsing logic, since my unit
// tests passed with Go 1.1 and the App Engine SDK is Go 1.0,
// but the SDK's "go test" is broken. Yay. So test this way,
// rather than installing Go 1.0.
func init() {
	dat := `<?xml version='1.0' encoding='UTF-8'?>
<feed xmlns='http://www.w3.org/2005/Atom' xmlns:gd='http://schemas.google.com/g/2005' xmlns:issues='http://schemas.google.com/projecthosting/issues/2009'>

<entry gd:etag='W/&quot;DUANRX47eCl7ImA9WhBaFUk.&quot;'>
   <id>http://code.google.com/feeds/issues/p/go/issues/full/5128</id>
   <published>2013-03-25T22:26:38.000Z</published>
   <updated>2013-05-26T05:56:34.000Z</updated>
   <title>cmd/gofmt reformats block comments</title>
   <author>
     <name>bor...@google.com</name>
     <uri>/u/106051431195682631099/</uri>
   </author>
   <issues:owner>
     <issues:uri>/u/102602228801687104398/</issues:uri>
     <issues:username>g...@golang.org</issues:username>
   </issues:owner>
</entry>
</feed>`
	issues, err := ParseIssues(strings.NewReader(dat))
	if err != nil {
		panic(err)
	}
	if len(issues) < 1 {
		panic("no issues parsed")
	}
	if issues[0].Owner == nil {
		panic("nil Owner parsed")
	}
	if issues[0].Owner.URI != "/u/102602228801687104398/" {
		panic("no URI in Owner")
	}
}
