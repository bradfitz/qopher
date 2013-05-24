// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package task

import (
	"net/http"
	"sort"
	"time"
)

// A PolledTask is an open task as returned by Type.Poll.
// It isn't written to the datastore as-is, but is converted
// to a new or existing qopher.Task.
type PolledTask struct {
	ID    string // without the "type." prefix.
	Date  time.Time
	Title string
	Body  []byte // if any
}

func (pt *PolledTask) DateOrNow() time.Time {
	if pt.Date.IsZero() {
		return time.Now()
	}
	return pt.Date
}

// Type is a task type.
type Type interface {
	Type() string                // "issue"
	PollInterval() time.Duration // minute granularity
	Poll(Env) ([]*PolledTask, error)
	TaskURL(id string) string
}

// An Env is the environment a task Type needs to do its work.
type Env interface {
	HTTPClient() *http.Client
	Logf(format string, args ...interface{})
}

// TypeMap contains the types registered by RegisterType.
// Do not modify.
var TypeMap = make(map[string]Type)

// Types are the sorted task types.
// Do not modify. This is updated by RegisterType.
var Types []Type

func RegisterType(tt Type) {
	key := tt.Type()
	if _, dup := TypeMap[key]; dup {
		panic("duplicate registration of task type " + key)
	}
	TypeMap[key] = tt
	keys := make([]string, 0, len(TypeMap))
	for key := range TypeMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	Types = nil
	for _, key := range keys {
		Types = append(Types, TypeMap[key])
	}
}
