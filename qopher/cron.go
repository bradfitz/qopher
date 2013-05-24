// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/delay"

	"qopher/task"
)

func init() {
	http.HandleFunc("/cron/poll", cronPoll)
}

func cronPoll(rw http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	var wg sync.WaitGroup

	wg.Add(1)
	var m map[string]PollResult
	go func() {
		defer wg.Done()
		m = Poll(&appengineEnv{ctx}, 25*time.Second, task.Types)
	}()

	q := datastore.NewQuery("Task").
		Filter("Closed = ", false).
		KeysOnly()
	keys, err := q.GetAll(ctx, nil)
	if err != nil {
		ctx.Errorf("GetAll failed: %v", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	ctx.Infof("Keys open = %q", keys)
	wg.Wait()

	inDatastore := make(map[string]bool) // "codereview.1234" => true
	for _, key := range keys {
		strKey := key.StringID()
		strKey = strings.TrimLeft(strKey, "/")  // WTF?
		inDatastore[strKey] = true
	}

	var ops []string
	noOps, adds, deletes := 0, 0, 0

	for _, tt := range task.Types {
		typeStr := tt.Type()
		res := m[typeStr]
		if res.Err != nil {
			ops = append(ops, fmt.Sprintf("Type %s poll failure: %v", res.Type, res.Err))
			ctx.Errorf("Failed to poll %q: %v", res.Type, res.Err)
			// Ignore that these exist in the datastore for now.
			typeDot := typeStr + "."
			for key := range inDatastore {
				if strings.HasPrefix(key, typeDot) {
					delete(inDatastore, key)
				}
			}
			continue
		}
		for _, pt := range res.Tasks {
			typeID := typeStr + "." + pt.ID
			if inDatastore[typeID] {
				ops = append(ops, fmt.Sprintf("Already open: %s", typeID))
				// Common case; all good.
				noOps++
				delete(inDatastore, typeID)
				continue
			}
			ops = append(ops, fmt.Sprintf("Add: %s", typeID))
			adds++
			wg.Add(1)
			go func(pt *task.PolledTask) {
				defer wg.Done()
				k := datastore.NewKey(ctx, "Task", typeID, 0, nil)
				task := &Task{
					Owner:    "", // To be assigned later.
					Type:     typeStr,
					ID:       pt.ID,
					Created:  pt.DateOrNow(),
					Modified: time.Now(),
				}
				_, err := datastore.Put(ctx, k, task)
				if err != nil {
					// Oh well. It'll happen next cron. But at least log:
					ctx.Errorf("cron: Error inserting task %q: %v", typeID, err)
				}
			}(pt)
		}
	}

	// Unassign things which are no longer open, but exist in the datastore.
	for typeID := range inDatastore {
		ops = append(ops, fmt.Sprintf("Delete: %s", typeID))
		deletes++
		wg.Add(1)
		go func(typeID string) {
			defer wg.Done()
			closeTask.Call(ctx, typeID)
			ctx.Infof("scheduled task %q for close", typeID)
		}(typeID)
	}

	wg.Wait()

	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(rw, "noops=%d; adds=%d; deletes=%d\n\n", noOps, adds, deletes)
	for _, op := range ops {
		fmt.Fprintf(rw, "%s\n", op)
	}
}

var closeTask = delay.Func("close_task", func(c appengine.Context, typeID string) error {
	key := datastore.NewKey(c, "Task", typeID, 0, nil)
	return datastore.RunInTransaction(c, func(c appengine.Context) error {
		task := new(Task)
		err := datastore.Get(c, key, task)
		if err != nil {
			return err
		}
		task.Closed = true
		task.Owner = ""
		_, err = datastore.Put(c, key, task)
		return err
	}, nil)
})