// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"fmt"
	"net/http"
	"time"

	"appengine"
	"appengine/urlfetch"

	_ "qopher/task/issue"
)

// A Task is something that a human needs to do.
//
// It is assigned to exactly one person at a time, but anybody
// can steal it, close it, or reassign it.
//
// A Task might be:
//
// * doing an incoming codereview
// * triaging an Issue Tracker bug. Something from:
//     https://code.google.com/p/go/issues/list?can=2&q=Priority%3DTriage+&cells=tiles
// * looking at a build breakage. A failure at:
//     http://build.golang.org/
// * moderating an email to golang-nuts or golang-dev:
//     https://groups.google.com/forum/?hl=en_US&fromgroups#!pendingmsg/golang-dev
//     https://groups.google.com/forum/?hl=en_US&fromgroups#!pendingmsg/golang-nuts
type Task struct {
	TypeAndID string // "issue.4944", "cl.9668043", "moderate.gonuts"
	Title     string `datastore:",noindex"`
	Owner     string // GAE email address (remapped to display name later), or "" if closed
	Created   time.Time
	Modified  time.Time
	Assigned  time.Time
}

// A LogEntry records the creation, reassignment, and closing of Tasks.
type LogEntry struct {
	Time      time.Time
	TypeAndID string
	Action    string // "new", "close", "reassign", etc
	Who       string // who performed the action, or "" for the system itself
	WhoTo     string // recipient
}

type Person struct {
	Likes    []string
	Dislikes []string
	Vacation bool
}

type appengineEnv struct {
	ctx appengine.Context
}

func (e *appengineEnv) HTTPClient() *http.Client {
	return urlfetch.Client(e.ctx)
}

func init() {
	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(rw, "It is %v.", time.Now())
	})
}
