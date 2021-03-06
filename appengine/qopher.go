// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"fmt"
	"net/http"
	"runtime"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/urlfetch"

	_ "qopher/task/codereview"
	_ "qopher/task/issue"
	// TODO: implement these:
	//_ "qopher/task/moderate"   // moderate golang-nuts and golang-dev
	//_ "qopher/task/buildbreak" // investigate build.golang.org breakages
)

var isGo11 bool

func init() {
	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		// Oh, https://code.google.com/p/go/issues/detail?id=4799
		if r.URL.Path == "/" {
			serveFront(rw, r)
		} else {
			http.NotFound(rw, r)
		}
	})

	http.HandleFunc("/go11", func(rw http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(rw, "isGo11 = %v; version = %v", isGo11, runtime.Version())
	})
}

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
//
// Keyed in datastore by "<Type>.<ID>"
type Task struct {
	Type     string // "issue", "codereview", "moderate", ...
	ID       string // "4944", "gonuts", ...
	Closed   bool
	Title    string `datastore:",noindex"`
	Owner    string // GAE email address (remapped to display name later), or "" if closed
	Created  time.Time
	Modified time.Time
	Assigned time.Time
}

// A LogEntry records the creation, reassignment, and closing of Tasks.
type LogEntry struct {
	Time    time.Time
	TaskKey string // "<type>.<id>"
	Action  string // "new", "close", "reassign", etc
	Who     string // who performed the action, or "" for the system itself
	WhoTo   string // recipient
}

// TODO(bradfitz): use this.
type Person struct {
	Likes    []string
	Dislikes []string
	Vacation bool
}

type appengineEnv struct {
	ctx appengine.Context
}

func (e *appengineEnv) HTTPClient() *http.Client {
	return &http.Client{
		Transport: &urlfetch.Transport{
			Context:  e.ctx,
			Deadline: 30 * time.Second,
		},
	}
}

func (e *appengineEnv) Logf(format string, args ...interface{}) {
	e.ctx.Infof(format, args...)
}

type meta struct {
	Value []byte `datastore:",noindex"`
}

func (e *appengineEnv) GetMeta(key string) ([]byte, error) {
	var m meta
	k := datastore.NewKey(e.ctx, "Meta", key, 0, nil)
	err := datastore.Get(e.ctx, k, &m)
	if err == datastore.ErrNoSuchEntity {
		return nil, nil
	}
	return m.Value, err
}

func (e *appengineEnv) SetMeta(key string, value []byte) error {
	k := datastore.NewKey(e.ctx, "Meta", key, 0, nil)
	_, err := datastore.Put(e.ctx, k, &meta{Value: value})
	return err
}

func getMethod(r *http.Request) bool {
	return r.Method == "GET" || r.Method == "HEAD"
}
