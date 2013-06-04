// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package task

import (
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"
)

// A PolledTask is an open task as returned by Type.Poll.
// It isn't written to the datastore as-is, but is converted
// to a new or existing qopher.Task.
type PolledTask struct {
	ID        string // without the "type." prefix.
	Date      time.Time
	Title     string
	OwnerHint string // optional. prefix or email address.
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

	// GetMeta returns a blob up to 1MB in size. The key's
	// namespace is shared with all task types, so should
	// be prefixed.
	// If nothing is set, the return value is (nil, nil).
	GetMeta(key string) ([]byte, error)

	// SetMeta sets a blob up to 1MB in size. The key's
	// namespace is shared with all task types, so should
	// be prefixed.
	SetMeta(key string, value []byte) error
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

// devAppServer returns whether we're on the devAppServer, without
// importing the "appengine" package, so this file can still be
// tested.
func devAppServer() bool {
	i := 0
	for {
		i++
		_, file, _, ok := runtime.Caller(i)
		if !ok {
			break
		}
		if strings.HasPrefix(file, "/Users") || strings.HasPrefix(file, "/home/") {
			return true
		}
	}
	return false
}
