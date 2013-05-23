// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"sync"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/user"

	"qopher/task"
)

func init() {
	http.HandleFunc("/admin/add", adminAdd)
	http.HandleFunc("/admin/polldebug", adminPollDebug)
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
		Owner:     u.Email,
		TypeAndID: typeId,
		Created:   now,
		Modified:  now,
		Assigned:  now,
	}
	_, err := datastore.Put(ctx, k, task)
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(rw, "put of %q = %v", typeId, err)
}

func adminPollDebug(rw http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)

	type res struct {
		Type  string
		Tasks []*task.PolledTask
		Err   error
	}
	env := &appengineEnv{ctx}
	var mmu sync.Mutex // guards m
	m := make(map[string]res)
	var wg sync.WaitGroup
	for _, tt := range task.Types {
		wg.Add(1)
		go func(tt task.Type) {
			defer wg.Done()
			resc := make(chan res, 1)
			go func() {
				tasks, err := tt.Poll(env)
				resc <- res{tt.Type(), tasks, err}
			}()
			var v res
			select {
			case v = <-resc:
			case <-time.After(5 * time.Second):
				v = res{tt.Type(), nil, errors.New("timeout")}
			}
			mmu.Lock()
			defer mmu.Unlock()
			m[tt.Type()] = v

		}(tt)
	}
	wg.Wait()

	var page struct {
		Types []res
	}
	for _, tt := range task.Types {
		page.Types = append(page.Types, m[tt.Type()])
	}
	frontPage.ExecuteTemplate(rw, "front", &page)
}

var frontPage = template.Must(template.New("front").Funcs(template.FuncMap{
	"selected": func(a, b string) string {
		if a == b {
			return "selected"
		}
		return ""
	},
}).Parse(`
<!doctype html>
<html>
  <head>
    <title>qopher poll debug</title>
  </head>
  <body>

<h1>qopher task poll debug</h1>

{{range $i, $tt := .Types}}
<h2>{{$tt.Type}}</h2>
  {{if $tt.Err}}
     <b>Error:</b> {{$tt.Err}}
  {{else}}
    {{range $i, $task := $tt.Tasks}}
       <p><b>[ID {{$task.ID}}]</b> - {{$task.Summary}}</p>
    {{end}}
  {{end}}

{{end}}

  </body>
</html>
`))
