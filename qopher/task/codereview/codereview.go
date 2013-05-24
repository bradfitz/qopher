// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codereview

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"qopher/task"
)

const (
	urlPrefix = "https://codereview.appspot.com/"
	query     = "https://codereview.appspot.com/search?closed=3&owner=&reviewer=golang-dev%40googlegroups.com&repo_guid=&base=&private=1&created_before=&created_after=2012-01-01+00%3A00%3A00&modified_before=&modified_after=&order=created&format=html&keys_only=False&with_messages=False&cursor=&limit=30&format=json"
)

type codereviewTask struct{}

func init() {
	task.RegisterType(codereviewTask{})
}

func (codereviewTask) TaskURL(id string) string    { return urlPrefix + id }
func (codereviewTask) Type() string                { return "codereview" }
func (codereviewTask) PollInterval() time.Duration { return 5 * time.Minute }

func (codereviewTask) Poll(env task.Env) ([]*task.PolledTask, error) {
	c := env.HTTPClient()
	res, err := c.Get(query)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	reviews, cursor, err := ParseReviews(res.Body)
	if err != nil {
		return nil, err
	}
	if cursor != "" {
		panic("TODO: cursor")
	}
	env.Logf("got %d reviews", len(reviews))
	tasks := make([]*task.PolledTask, 0, len(reviews))
	for _, r := range reviews {
		if r.Issue == 0 {
			env.Logf("bogus issue %+v", r)
			continue
		}
		t := &task.PolledTask{
			ID:    fmt.Sprint(r.Issue),
			Title: r.Desc,
		}
		tasks = append(tasks, t)
	}
	env.Logf("returning %d tasks", len(tasks))
	return tasks, nil
}

type doc struct {
	Cursor  string    `json:"cursor"`
	Results []*Review `json:"results"`
}

// Time unmarshals a time in rietveld's format.
type Time time.Time

func (t *Time) UnmarshalJSON(b []byte) error {
	if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
		return fmt.Errorf("types: failed to unmarshal non-string value %q as an RFC 3339 time")
	}
	// TODO: pic
	tm, err := time.Parse("2006-01-02 15:04:05", string(b[1:len(b)-1]))
	if err != nil {
		return err
	}
	*t = Time(tm)
	return nil
}

func (t Time) String() string { return time.Time(t).String() }

type Review struct {
	Issue      int    `json:"issue"`
	Desc       string `json:"description"`
	OwnerEmail string `json:"owner_email"`
	Owner      string `json:"owner"`
	Created    Time   `json:"created"`
	Modified   Time   `json:"modified"`
}

func ParseReviews(r io.Reader) (reviews []*Review, cursor string, err error) {
	var d doc
	err = json.NewDecoder(r).Decode(&d)
	return d.Results, d.Cursor, err
}
