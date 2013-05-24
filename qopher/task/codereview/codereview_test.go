// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package codereview

import (
	"os"
	"testing"
	"flag"
	"net/http"
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
