// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package issue

import (
	"testing"
	"io/ioutil"
	"bytes"
)

func TestParseIssues(t *testing.T) {
	slurp, err := ioutil.ReadFile("testdata/triage.xml")
	if err != nil {
		t.Fatal(err)
	}
	issues, err := ParseIssues(bytes.NewReader(slurp))
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) < 2 {
		t.Error("no issues parsed")
	}
	for _, is := range issues {
		t.Logf("Issue: %+v", is)
	}
}


