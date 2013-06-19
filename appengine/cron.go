// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

import (
	"bytes"
	"fmt"
	"io"
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
	http.HandleFunc("/cron/assign", cronAssign)
}

type pollState struct {
	ctx            appengine.Context
	open           map[string]*Task // known & open in datastore. keys like "codereview.1234"
	pollFailPrefix []string         // prefixes of task types that failed to poll ("codereview.")

	wg sync.WaitGroup // for task update/delete/insert operations

	// Summary for HTTP response
	noops, adds, updates, deletes int
	ops                           []string // human readable

	pollwg   sync.WaitGroup
	pollres  map[string]PollResult
	pollopen map[string]bool // "codereview.1234" => true
}

func (ps *pollState) startPoll() {
	ps.pollwg.Add(1)
	go func() {
		defer ps.pollwg.Done()
		defer ps.ctx.Infof("Poll complete.")
		ps.ctx.Infof("Poll starting...")
		ps.pollres = Poll(&appengineEnv{ps.ctx}, 25*time.Second, task.Types)
		ps.pollopen = make(map[string]bool)
		for typeStr, res := range ps.pollres {
			if res.Err != nil {
				ps.pollFailPrefix = append(ps.pollFailPrefix, res.Type+".")
				continue
			}
			for _, pt := range res.Tasks {
				typeID := typeStr + "." + pt.ID
				ps.pollopen[typeID] = true
			}
		}
	}()
}

func (ps *pollState) startCloseTask(typeID string) {
	ps.wg.Add(1)
	ps.ops = append(ps.ops, fmt.Sprintf("Delete: %s", typeID))
	ps.deletes++
	go func() {
		defer ps.wg.Done()
		closeTask.Call(ps.ctx, typeID)
		ps.ctx.Infof("scheduled task %q for close", typeID)
	}()
}

func (ps *pollState) startAddTask(typeStr, typeID string, pt *task.PolledTask) {
	ps.wg.Add(1)
	ps.adds++
	ps.ops = append(ps.ops, fmt.Sprintf("Add: %s", typeID))
	go func() {
		defer ps.wg.Done()
		k := datastore.NewKey(ps.ctx, "Task", typeID, 0, nil)
		task := &Task{
			Owner:    "", // To be assigned later.
			Type:     typeStr,
			ID:       pt.ID,
			Created:  pt.GetCreated(),
			Modified: pt.GetModified(),
			Title:    pt.Title,
		}
		_, err := datastore.Put(ps.ctx, k, task)
		if err != nil {
			// Oh well. It'll happen next cron. But at least log:
			ps.ctx.Errorf("cron: Error inserting task %q: %v", typeID, err)
		}
	}()
}

func (ps *pollState) startUpdateTaskOwner(typeStr, typeID string, pt *task.PolledTask, newOwner string) {
	ps.wg.Add(1)
	go func() {
		defer ps.wg.Done()
		taskAssignTo.Call(ps.ctx, typeID, newOwner)
	}()
}

func (ps *pollState) loadOpenTasks() error {
	var tasks []*Task
	q := datastore.NewQuery("Task").Filter("Closed = ", false)
	keys, err := q.GetAll(ps.ctx, &tasks)
	if err != nil {
		return err
	}
	ps.ctx.Infof("Keys open = %q", keys)
	ps.open = make(map[string]*Task)
	for i, key := range keys {
		ps.open[key.StringID()] = tasks[i]
	}
	return nil
}

// pollFailed returns whether the poll type for the given typeID
// ("codereview.1234") failed.
func (ps *pollState) pollFailed(typeID string) bool {
	for _, pfx := range ps.pollFailPrefix {
		if strings.HasPrefix(typeID, pfx) {
			return true
		}
	}
	return false
}

func (ps *pollState) updateTask(typeStr, typeID string, pt *task.PolledTask) {
	task := ps.open[typeID]
	if task == nil {
		ps.startAddTask(typeStr, typeID, pt)
		return
	}
	newOwner := mapEmail(pt.OwnerHint)
	if pt.OwnerHint == "" || newOwner == task.Owner {
		// Common case: already open, no owner change.
		ps.noops++
		ps.ops = append(ps.ops, fmt.Sprintf("Already open: %s", typeID))
		return
	}
	ps.updates++
	ps.ops = append(ps.ops, fmt.Sprintf("Changing owner of %s from %q to %q", typeID, task.Owner, newOwner))
	ps.startUpdateTaskOwner(typeStr, typeID, pt, newOwner)
}

func (ps *pollState) tasksToClose() (typeIDs []string) {
	for typeID := range ps.open {
		if ps.pollFailed(typeID) {
			continue
		}
		if !ps.pollopen[typeID] {
			typeIDs = append(typeIDs, typeID)
		}
	}
	return
}

// updateDatastore waits for startPoll to finish, and then compares
// what's in the datastore with the poll results and issues datastore
// updates as necessary to add/remove/update tasks.
func (ps *pollState) updateDatastore() {
	ps.pollwg.Wait()
	defer ps.wg.Wait()

	for _, tt := range task.Types {
		typeStr := tt.Type()
		res := ps.pollres[typeStr]
		if res.Err != nil {
			ps.ops = append(ps.ops, fmt.Sprintf("Type %s poll failure: %v", res.Type, res.Err))
			ps.ctx.Errorf("Failed to poll %q: %v", res.Type, res.Err)
			continue
		}
		for _, pt := range res.Tasks {
			typeID := typeStr + "." + pt.ID
			ps.updateTask(typeStr, typeID, pt)
		}
	}

	// Unassign things which are no longer open, but exist in the datastore.
	for _, typeID := range ps.tasksToClose() {
		ps.startCloseTask(typeID)
	}
}

func cronPoll(rw http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	ps := &pollState{ctx: ctx}
	ps.startPoll()
	if err := ps.loadOpenTasks(); err != nil {
		ctx.Errorf("loadOpenTasks failed: %v", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}

	ps.updateDatastore()

	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(rw, "noops=%d; adds=%d; deletes=%d\n\n", ps.noops, ps.adds, ps.deletes)
	for _, op := range ps.ops {
		fmt.Fprintf(rw, "%s\n", op)
	}
}

func cronAssign(rw http.ResponseWriter, r *http.Request) {
	ctx := appengine.NewContext(r)
	q := datastore.NewQuery("Task").
		Filter("Closed = ", false).
		Filter("Owner = ", "").
		KeysOnly()
	keys, err := q.GetAll(ctx, nil)
	if err != nil {
		ctx.Errorf("GetAll failed: %v", err)
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	ctx.Infof("Keys open = %q", keys)
	var buf bytes.Buffer
	var wg sync.WaitGroup
	for _, key := range keys {
		wg.Add(1)
		typeID := key.StringID()
		go func() {
			defer wg.Done()
			taskAssignSomebody.Call(ctx, typeID)
		}()
		fmt.Fprintf(&buf, "open, unassigned = %s\n", typeID)
	}
	wg.Wait()
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(rw, "%d assignments:\n", len(keys))
	io.Copy(rw, &buf)
}

// close the task because it's no longer open elsewhere.
var closeTask = delay.Func("close_task", func(c appengine.Context, typeID string) error {
	now := time.Now()
	key := datastore.NewKey(c, "Task", typeID, 0, nil)
	doLog := false
	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		task := new(Task)
		err := datastore.Get(c, key, task)
		if err != nil {
			return err
		}
		if task.Closed {
			return nil
		}
		task.Closed = true
		task.Owner = ""
		_, err = datastore.Put(c, key, task)
		doLog = true
		return err
	}, nil)
	if err == nil && doLog {
		key := datastore.NewIncompleteKey(c, "LogEntry", nil)
		_, err = datastore.Put(c, key, &LogEntry{
			Time:    now,
			TaskKey: typeID,
			Action:  "close",
		})
	}
	return err
})

var taskAssignSomebody = delay.Func("task_assign_anybody", func(c appengine.Context, typeID string) error {
	now := time.Now()
	key := datastore.NewKey(c, "Task", typeID, 0, nil)
	doLog := false
	victim := RandomVictimEmail()
	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		task := new(Task)
		err := datastore.Get(c, key, task)
		if err != nil {
			return err
		}
		if task.Owner != "" {
			return nil
		}
		task.Owner = victim
		task.Assigned = now
		_, err = datastore.Put(c, key, task)
		doLog = true
		return err
	}, nil)
	if err == nil && doLog {
		key := datastore.NewIncompleteKey(c, "LogEntry", nil)
		_, err = datastore.Put(c, key, &LogEntry{
			Time:    now,
			TaskKey: typeID,
			Action:  "assign",
			WhoTo:   victim,
		})
	}
	return err
})

var taskAssignTo = delay.Func("task_assign_to", func(c appengine.Context, typeID, victim string) error {
	now := time.Now()
	key := datastore.NewKey(c, "Task", typeID, 0, nil)
	doLog := false
	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		task := new(Task)
		err := datastore.Get(c, key, task)
		if err != nil {
			return err
		}
		task.Owner = mapEmail(victim)
		task.Assigned = now
		_, err = datastore.Put(c, key, task)
		doLog = true
		return err
	}, nil)
	if err == nil && doLog {
		key := datastore.NewIncompleteKey(c, "LogEntry", nil)
		_, err = datastore.Put(c, key, &LogEntry{
			Time:    now,
			TaskKey: typeID,
			Action:  "assign",
			WhoTo:   victim,
		})
	}
	return err
})
