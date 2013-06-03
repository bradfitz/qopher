// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codereview

import (
	"flag"
	"net/http"
	"os"
	"reflect"
	"testing"
	"time"
)

var live = flag.Bool("live", false, "Hit the network.")

type testEnv struct {
	t *testing.T
}

func (e testEnv) Logf(format string, args ...interface{}) {
	e.t.Logf(format, args...)
}

func (e testEnv) HTTPClient() *http.Client { return http.DefaultClient }

func TestLive(t *testing.T) {
	if !*live {
		t.Skipf("skipping")
	}
	var task codereviewTask
	tasks, err := task.Poll(testEnv{t})
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
	tests := []struct{
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
