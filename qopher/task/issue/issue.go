// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package issue

import (
	"encoding/xml"
	"io"
)

type feed struct {
	XMLName xml.Name `xml:"feed"`
	Entry   []*Issue `xml:"entry"`
}

type Issue struct {
	Id    string `xml:"id"`
	Title string `xml:"title"`
}

func ParseIssues(r io.Reader) ([]*Issue, error) {
	var f feed
	err := xml.NewDecoder(r).Decode(&f)
	return f.Entry, err
}
