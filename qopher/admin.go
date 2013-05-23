// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"fmt"
	"net/http"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/user"
)

func init() {
	http.HandleFunc("/admin/add", adminAdd)
}

func adminAdd(rw http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	u := user.Current(ctx)
	if u == nil || !u.Admin {
		http.Error(rw, "requires admin", 401)
		return
	}
	id := r.FormValue("id")
	if id == "" {
		http.Error(rw, "no id parameter", 400)
		return
	}
	typeId := "manual." + id
	k := datastore.NewKey(ctx, "Task", typeId, 0, nil)
	now := time.Now()
	task := &Task{
		Owner: u.Email,
		TypeAndID: typeId,
		Created: now,
		Modified: now,
		Assigned: now,
	}
	_, err := datastore.Put(ctx, k, task)
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(rw, "put of %q = %v", typeId, err)
}



