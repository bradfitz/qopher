// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"html/template"
	"net/http"

	"appengine"
	"appengine/datastore"
	"appengine/user"

	"qopher/task"
)

type FrontPage struct {
	NTotal          int
	QueueEmail      string // showing queue for this email address
	QueueEmailShort string
	Yours           []*Task
	Other           []*Task
}

func serveFront(rw http.ResponseWriter, r *http.Request) {
	if !getMethod(r) {
		http.Error(rw, "bad method", http.StatusMethodNotAllowed)
		return
	}
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		http.Error(rw, "login required", http.StatusUnauthorized)
		return
	}

	email := r.FormValue("q")
	if email == "" {
		email = u.Email
	}
	email = mapEmail(email)

	if email == "crash" {
		panic("fake crash")
	}

	q := datastore.NewQuery("Task").Filter("Closed = ", false)
	var tasks []*Task
	_, err := q.GetAll(c, &tasks)
	if err != nil {
		c.Errorf("GetAll: %v", err)
		http.Error(rw, err.Error(), 500)
		return
	}
	numIssues := len(tasks)

	var yours, other []*Task
	for _, t := range tasks {
		if t.Owner == email {
			yours = append(yours, t)
		} else {
			other = append(other, t)
		}
	}

	const maxOther = 50
	if len(other) > maxOther {
		other = other[:maxOther]
	}

	page := &FrontPage{
		NTotal:          numIssues,
		QueueEmail:      email,
		QueueEmailShort: emailToShort(email),
		Yours:           yours,
		Other:           other,
	}

	err = frontPage.ExecuteTemplate(rw, "front", page)
	if err != nil && r.Method != "HEAD" {
		c.Errorf("template error: %v", err)
	}
}

var frontPage = template.Must(template.New("front").Funcs(template.FuncMap{
	"taskURL": func(taskType, id string) string {
		return task.TypeMap[taskType].TaskURL(id)
	},
	"shortOwner": func(t *Task) string {
		if t.Owner == "" {
			return ""
		}
		return emailToShort(t.Owner) + ": "
	},
}).Parse(`
<!doctype html>
<html>
  <head>
    <title>qopher - Go queue</title>
  </head>
  <body>

<h1>qopher: the Gopher Queue</h1>

<h2 style='color: red'>WORK IN PROGRESS</h2>

<p>Total <b>{{$.NTotal}}</b> open tasks</a>

<p>Viewing queue for <b>{{$.QueueEmail}}</b> (change with ?q= nickname or email)</p>

{{if $.Yours}}
  <h2>Assigned to <i>{{$.QueueEmailShort}}</i> ({{len $.Yours}})</h2>
  <p>This list doesn't include your <a href="https://code.google.com/p/go/issues/list?can=3&q=&colspec=ID+Status+Stars+Priority+Owner+Reporter+Summary&cells=tiles">open and assigned issues</a>.</p>
  <ul>
  {{range $i, $t := $.Yours}}
    <li><a href="{{taskURL $t.Type $t.ID}}">{{$t.Type}}.{{$t.ID}}</a>: {{$t.Title}}</li>
  {{end}}
  </ul>
{{else}}
 <h2>Nothing assigned to <i>{{$.QueueEmail}}</i></h2>
  <p>Also check your <a href="https://code.google.com/p/go/issues/list?can=3&q=&colspec=ID+Status+Stars+Priority+Owner+Reporter+Summary&cells=tiles">open and assigned issues</a>.</p>
{{end}}

{{if $.Other}}
  <h2>Some other open tasks</h2>
  <ul>
  {{range $i, $t := $.Other}}
    <li><a href="{{taskURL $t.Type $t.ID}}">{{$t.Type}}.{{$t.ID}}</a>: <i>{{shortOwner $t}}</i> {{$t.Title}}</li>
  {{end}}
  </ul>
{{end}}

  </body>
</html>
`))
