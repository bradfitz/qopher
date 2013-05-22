// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"fmt"
	"net/http"
	"time"

	"appengine"
)

func init() {
	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(rw, "It is %v. dev_appserver=%v", time.Now(), appengine.IsDevAppServer())
	})
}
