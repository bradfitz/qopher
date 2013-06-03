// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"errors"
	"sync"
	"time"

	"qopher/task"
)

type PollResult struct {
	Type  string
	Tasks []*task.PolledTask
	Err   error
}

func Poll(env task.Env, timeout time.Duration, types []task.Type) map[string]PollResult {
	var mu sync.Mutex // guards m
	m := make(map[string]PollResult)
	var wg sync.WaitGroup
	for _, tt := range types {
		wg.Add(1)
		go func(tt task.Type) {
			defer wg.Done()
			resc := make(chan PollResult, 1)
			go func() {
				tasks, err := tt.Poll(env)
				resc <- PollResult{tt.Type(), tasks, err}
			}()
			var v PollResult
			select {
			case v = <-resc:
			case <-time.After(timeout):
				v = PollResult{tt.Type(), nil, errors.New("timeout")}
			}
			mu.Lock()
			defer mu.Unlock()
			m[tt.Type()] = v
		}(tt)
	}
	wg.Wait()
	return m
}
