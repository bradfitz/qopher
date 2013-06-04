// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"appengine"

	"qopher/task/codereview"
)

func init() {
	if appengine.IsDevAppServer() {
		codereview.NumMonths = 3
	}
}
