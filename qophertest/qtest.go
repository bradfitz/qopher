// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qophertest

import (
	"net/http"
	"testing"

	"qopher/task"
)

// Env is an implementation of qopher/task.Env for testing.
type Env struct {
	t    *testing.T
	meta map[string]string
}

var _ task.Env = (*Env)(nil)

func NewEnv(t *testing.T) *Env {
	return &Env{t, make(map[string]string)}
}

func (e *Env) Logf(format string, args ...interface{}) {
	e.t.Logf(format, args...)
}

func (e *Env) HTTPClient() *http.Client { return http.DefaultClient }

func (e *Env) SetMeta(key string, value []byte) error {
	e.meta[key] = string(value)
	return nil
}

func (e *Env) GetMeta(key string) ([]byte, error) {
	s, ok := e.meta[key]
	if !ok {
		return nil, nil
	}
	return []byte(s), nil
}
