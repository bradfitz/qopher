// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"html/template"
	"net/http"
	"strings"

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

	const maxOther = 200
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
		return emailToShort(t.Owner)
	},
	"snippet": func(v string) string {
		if i := strings.Index(v, "\n"); i > 0 {
			v = v[:i]
		}
		if len(v) > 100 {
			v = v[:100]
		}
		return v
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
  <p>Your job is to make these go away somehow. See <a href="#help">Help section</a> below.</p>
  <p>This list doesn't include your <a href="https://code.google.com/p/go/issues/list?can=3&q=&colspec=ID+Status+Stars+Priority+Owner+Reporter+Summary&cells=tiles">open and assigned issues</a>.</p>
  <ul>
  {{range $i, $t := $.Yours}}
    <li><a href="{{taskURL $t.Type $t.ID}}">{{$t.Type}}.{{$t.ID}}</a>: {{snippet $t.Title}}</li>
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
    <li><a href="{{taskURL $t.Type $t.ID}}">{{$t.Type}}.{{$t.ID}}</a>, for <i>{{shortOwner $t}}</i>: {{snippet $t.Title}}</li>
 {{end}}
  </ul>
{{end}}

<h2 id='help'>Help</h2>
<p>Your job is to make the items in the "Assigned to you" section at top go away.  You can make them go away by:</p>
<ul>
  <li> assign it to somebody more appropriate, either in the issue tracker or R= on codereview. In codereview, it is NOT necessary to send email. You can uncheck the "send email" checkbox if you don't want to spam.</li>
  <li> or close it (remove golang-dev as a reviewer, or R=close)</li>
  <li> mute it for awhile by commenting with a top-line of "Q=wait" on codereview. it will go away until there's a new comment.</li>
  <li> submit it or fix it, which closes the task here</li>
</ul>
<p>The list is updated once per minute, polled from the source of truth elsewhere.</p>
<p>If you're bored, you can clean up the "Some other open tasks" section, or just reassign things. Every open task has exactly 1 owner, but anybody can do anything.</p>
<p>This isn't a bug tracker. It's mostly a triage system, to make sure nothing is dropped and triage load is spread. If somebody filed a bug without an owner, it shows up here. Once somebody assigns it to you, go to the bug tracker to see <a href="https://code.google.com/p/go/issues/list?can=3&q=&colspec=ID+Status+Stars+Priority+Owner+Reporter+Summary&cells=tiles">the bugs assigned to you</a>.</p>

  </body>
</html>
`))
