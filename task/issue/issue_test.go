// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package issue

import (
	"bytes"
	"io/ioutil"
	"testing"
	"runtime"
)

func TestParseIssues(t *testing.T) {
	t.Logf("version = %s", runtime.Version())
	const minimal = false
	file := "triage.xml"
	if minimal {
		file = "minimal.xml"
	}
	slurp, err := ioutil.ReadFile("testdata/" + file)
	if err != nil {
		t.Fatal(err)
	}
	issues, err := ParseIssues(bytes.NewReader(slurp))
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Error("no issues parsed")
	}
	for _, is := range issues {
		t.Logf("Issue: %+v, owner=%#v", is, is.Owner)
	}
}
