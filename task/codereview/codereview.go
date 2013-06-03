// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codereview

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"qopher/task"
)

// See:
// https://code.google.com/p/rietveld/wiki/APIs
// https://codereview.appspot.com/search (search form to make search URLs)
// https://code.google.com/p/rietveld/source/browse/index.yaml (indexes)

const (
	urlPrefix = "https://codereview.appspot.com/"

	maxItemsPerPage = 1000

	// Not closed queries, created after some point in time.
	// (closed=3 means not closed; private=1 means unspecified privacy)
	// dates are like 2012-08-01+00%3A00%3A00
	monthQuery = "https://codereview.appspot.com/search?closed=3&reviewer=golang-dev%40googlegroups.com&private=1&created_before={{CREATED_BEFORE}}&created_after={{CREATED_AFTER}}&order=created&keys_only=False&with_messages=False&cursor={{CURSOR}}&limit={{LIMIT}}&format=json"

	// JSON with the text of messages. e.g.
	// https://codereview.appspot.com/api/6454085?messages=true
	messagesQuery = "https://codereview.appspot.com/api/{{ISSUE_NUMBER}}?messages=true"

	// policyVersion is incremented whenever we change our policy on how we interpret
	// the comments in rietveld's meaning as to whether the issue is logically closed.
	//
	// The policy is currently:
	//   R=close (anybody can close it)
	//   R=bob (back open, with hint to assign it to bob)
	policyVersion = 1
)

type codereviewTask struct{}

func init() {
	task.RegisterType(codereviewTask{})
}

func (codereviewTask) TaskURL(id string) string    { return urlPrefix + id }
func (codereviewTask) Type() string                { return "codereview" }
func (codereviewTask) PollInterval() time.Duration { return 5 * time.Minute }

// General Poll strategy and notes:
//
// * in parallel, get all open issues by doing a 1000-results-per-page
//   query for each "created" month, going back max 1 or 2 years.
//   also in parallel, fetch (from qopher's memcache or datastore)
//   that month's last-polled cache metadata. So this is a total of 24
//   * 2 RPCs, but there may be more if a month has over 1000 created
//   issues, or there are memcache misses.
//
// * just because it's still "Open" to rietveld, doesn't mean it's
//   open as far as qopher is concerned. we also scan the comments of
//   each issue to determine whether it's logically closed. setting
//   "with_messages=True" on the per-month queries is too slow, and
//   usually returns useless and redundant data. instead, we only
//   fetch comments when we detect from the issue-level metadata that
//   something's changed. we store that the last modified time we saw
//   in a per-month datastructure in memcache/datastore that looks
//   like:
//
//   key "2013-05" value: "\n"-separated lines of:
//   "issue=6737064,modified=2012-10-28 10:23:18.058320,v=1,closed=true"
//   ... one per issue in that month.
//
// * for every rietveld-open issue in a month, compare its "modified"
//   JSON time to the "modified" field in our cache. If they differ
//   (string-wise), or if the policyVersion doesn't match, re-fetch
//   the comments of that issue and rebuild that month cache.

func (codereviewTask) Poll(env task.Env) (pt []*task.PolledTask, err error) {
	type res struct {
		pt  []*task.PolledTask
		err error
	}
	var chs []chan res
	for _, month := range relevantMonths() {
		ch := make(chan res, 1)
		chs = append(chs, ch)
		go func(month string) {
			pt, err := pollMonth(env, month)
			env.Logf("Month %q = %d, %v", month, len(pt), err)
			ch <- res{pt, err}
		}(month)
	}
	for _, ch := range chs {
		res := <-ch
		if res.err != nil {
			return nil, res.err
		}
		pt = append(pt, res.pt...)
	}
	return pt, nil
}

var nowFunc = time.Now

// relevantMonths returns the past n months in "yyyy-mm" format.
// These are the months to query for open issues.
func relevantMonths() []string {
	const nMonths = 12
	months := make([]string, nMonths)
	year, mon, _ := nowFunc().Date()
	for i := 0; i < nMonths; i++ {
		months[i] = fmt.Sprintf("%04d-%02d", year, int(mon))
		mon--
		if mon == 0 {
			mon = time.December
			year--
		}
	}
	sort.Strings(months)
	return months
}

var urlParam = regexp.MustCompile(`{{\w+}}`)

func urlWithParams(urlTempl string, m map[string]string) string {
	return urlParam.ReplaceAllStringFunc(urlTempl, func(param string) string {
		return url.QueryEscape(m[strings.Trim(param, "{}")])
	})
}

// "2013-01" -> "2013-02"
func monthAfter(month string) string {
	var yyyy, mm int
	fmt.Sscanf(month, "%04d-%02d", &yyyy, &mm)
	mm++
	if mm == 13 {
		mm = 1
		yyyy++
	}
	return fmt.Sprintf("%04d-%02d", yyyy, mm)
}

// itemsPerPage is the number of items to fetch for a single page.
// Changed by tests.
var itemsPerPage = maxItemsPerPage

func pollMonth(env task.Env, month string) (pt []*task.PolledTask, err error) {
	c := env.HTTPClient()
	cursor := ""
	n := 0
	for {
		url := urlWithParams(monthQuery, map[string]string{
			"CREATED_AFTER":  month + "-01 00:00:00",
			"CREATED_BEFORE": monthAfter(month) + "-01 00:00:00",
			"CURSOR": cursor,
			"LIMIT": fmt.Sprint(itemsPerPage),
		})
		n++
		env.Logf("Fetching codereview page %d: %s", n, url)
		res, err := c.Get(url)
		if err != nil {
			env.Logf("Error fetching %s: %v", url, err)
			return nil, err
		}
		var reviews []*Review
		reviews, cursor, err = ParseReviews(res.Body)
		res.Body.Close()
		if err != nil {
			env.Logf("ParseReviews error: %v", err)
			return nil, err
		}
		env.Logf("got %d reviews, cursor is %q", len(reviews), cursor)
		for _, r := range reviews {
			if r.Issue == 0 {
				env.Logf("bogus issue %+v", r)
				continue
			}
			t := &task.PolledTask{
				ID:    fmt.Sprint(r.Issue),
				Title: r.Desc,
			}
			pt = append(pt, t)
		}
		if cursor == "" || len(reviews) < itemsPerPage {
			break
		}
	}
	env.Logf("returning %d tasks", len(pt))
	return pt, nil
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
