// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codereview

import (
	"bytes"
	"flag"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"qopher/qophertest"
)

var live = flag.Bool("live", false, "Hit the network.")

func TestLive(t *testing.T) {
	if !*live {
		t.Skipf("skipping")
	}
	var task codereviewTask
	tasks, err := task.Poll(qophertest.NewEnv(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Got %d tasks", len(tasks))
	for _, task := range tasks {
		t.Logf("Task: %+v", task)
	}
}

func TestParse(t *testing.T) {
	f, err := os.Open("testdata/search.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	reviews, cursor, err := ParseReviews(f)
	if err != nil {
		t.Fatal(err)
	}
	if cursor == "" {
		t.Error("no cursor")
	}
	if want := 30; len(reviews) != want {
		t.Errorf("num reviews = %d; want %d", len(reviews), want)
	}
	for _, r := range reviews {
		t.Logf("Got %+v", r)
	}
}

func TestRelevantMonths(t *testing.T) {
	nowFunc = func() time.Time {
		return time.Unix(1370298894, 0)
	}
	got := relevantMonths()
	want := []string{"2012-07", "2012-08", "2012-09", "2012-10", "2012-11", "2012-12", "2013-01", "2013-02", "2013-03", "2013-04", "2013-05", "2013-06"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("relevantMonths = %q; want %q", got, want)
	}
}

func TestMonthAfter(t *testing.T) {
	tests := []struct {
		in, out string
	}{
		{"2011-12", "2012-01"},
		{"2011-04", "2011-05"},
	}
	for _, tt := range tests {
		got := monthAfter(tt.in)
		if got != tt.out {
			t.Errorf("monthAfter(%q) = %q; want %q", tt.in, got, tt.out)
		}
	}
}

func TestSummarizeIssue(t *testing.T) {
	f, err := os.Open("testdata/issue-9910043-msgs.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	env := qophertest.NewEnv(t)
	rt := qophertest.NewFakeRoundTripper(t)
	rt.AddURL("https://codereview.appspot.com/api/9910043?messages=true", &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       f,
	})
	env.SetHTTPClient(&http.Client{Transport: rt})
	im := summarizeIssue(env, 9910043)
	if im.err != nil {
		t.Fatalf("Error summarizing: %v", im.err)
	}
	want := issueMeta{
		issue:         9910043,
		lastModified:  "2013-06-03 19:50:42.516010",
		policyVersion: 1,
		reviewer:      "dvyukov",
	}
	if im != want {
		t.Errorf("summary = %+v; want %+v", im, want)
	}
}

func TestParseMonthMeta(t *testing.T) {
	mm, err := parseMonthMeta(strings.NewReader(`
issue=123,modified=2012-10-28 10:23:18.058320,v=1,r=
issue=1234,modified=2013-01-02 10:23:18.058320,v=2,r=close
issue=12345,modified=2013-03-03 10:23:18.058320,v=3,r=foo
`[1:]))
	if err != nil {
		t.Fatal(err)
	}
	want := monthMeta{
		123: issueMeta{
			issue:         123,
			lastModified:  "2012-10-28 10:23:18.058320",
			policyVersion: 1,
			reviewer:      "",
		},
		1234: issueMeta{
			issue:         1234,
			lastModified:  "2013-01-02 10:23:18.058320",
			policyVersion: 2,
			reviewer:      "close",
		},
		12345: issueMeta{
			issue:         12345,
			lastModified:  "2013-03-03 10:23:18.058320",
			policyVersion: 3,
			reviewer:      "foo",
		},
	}
	if !reflect.DeepEqual(mm, want) {
		t.Fatalf("Results differ:\n Got %#v\n\nWant %#v", mm, want)
	}

	back, err := parseMonthMeta(bytes.NewReader(mm.serialize()))
	if err != nil {
		t.Fatalf("From serialized:\n\n%s\n\n... error parsing it back: %v", mm.serialize(), err)
	}
	if !reflect.DeepEqual(back, want) {
		t.Fatalf("Parsing of serialization results differs from input:\n Got %#v\n\nWant %#v", back, want)
	}
}
