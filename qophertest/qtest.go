// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qophertest

import (
	"fmt"
	"net/http"
	"testing"

	"qopher/task"
)

// Env is an implementation of qopher/task.Env for testing.
type Env struct {
	t    *testing.T
	meta map[string]string

	// If nil, http.DefaultClient is used.
	client *http.Client
}

var _ task.Env = (*Env)(nil)

func NewEnv(t *testing.T) *Env {
	return &Env{t: t, meta: make(map[string]string)}
}

func (e *Env) Logf(format string, args ...interface{}) {
	e.t.Logf(format, args...)
}

func (e *Env) HTTPClient() *http.Client {
	if c := e.client; c != nil {
		return c
	}
	return http.DefaultClient
}

// SetHTTPClient sets the http client to use for testing. If nil,
// http.DefaultClient is used.
func (e *Env) SetHTTPClient(c *http.Client) {
	e.client = c
}

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

type RoundTripper struct {
	t *testing.T
	m map[string]*http.Response
}

func NewFakeRoundTripper(t *testing.T) *RoundTripper {
	return &RoundTripper{t, make(map[string]*http.Response)}
}

func (rt *RoundTripper) AddURL(url string, res *http.Response) {
	rt.m[url] = res
}

func (rt *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	res, ok := rt.m[url]
	if !ok {
		return nil, fmt.Errorf("qophertest: URL %q not registered in fake RoundTripper", url)
	}
	return res, nil
}
