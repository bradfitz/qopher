// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package qopher

// This file handles identities of people.

import (
	"math/rand"
	"sort"
	"strings"
	"time"
)

var (
	emailToPerson  = make(map[string]string) // email => person
	preferredEmail = make(map[string]string) // person => email
	personList     []string

	// People who get assigned tasks.
	victims = []string{
		"adg",
		"bradfitz",
		"campoy",
		"dsymonds",
		"gri",
		"iant",
		"khr",
		"nigeltao",
		"r",
		"rsc",
	}
)

func emailToShort(e string) string {
	if p, ok := emailToPerson[e]; ok {
		return p
	}
	i := strings.Index(e, "@")
	if i > 0 {
		return e[:i]
	}
	return "???"
}

// Converts either a person "foo" to an email address, or foo@google.com to foo@golang.org if it exists.
func mapEmail(e string) string {
	const suffix = "@google.com"
	if strings.HasSuffix(e, suffix) {
		prefix := e[:len(e)-len(suffix)]
		if pref, ok := preferredEmail[prefix]; ok {
			return pref
		}
	}
	if !strings.Contains(e, "@") {
		if pref, ok := preferredEmail[e]; ok {
			return pref
		}
	}
	return e
}

func RandomVictimEmail() string {
	return preferredEmail[victims[rand.Intn(len(victims))]]
}

func init() {
	rand.Seed(time.Now().UnixNano())

	// People we assume have golang.org and google.com accounts,
	// and prefer to use their golang.org address for code review.
	gophers := [...]string{
		"adg",
		"agl",
		"bradfitz",
		"campoy",
		"cshapiro",
		"dsymonds",
		"gri",
		"iant",
		"khr",
		"mpvl",
		"nigeltao",
		"r",
		"rsc",
		"sameer",
	}
	addGopher := func(p string) {
		personList = append(personList, p)
		emailToPerson[p+"@golang.org"] = p
		emailToPerson[p+"@google.com"] = p
		preferredEmail[p] = p + "@golang.org"
	}
	for _, p := range gophers {
		addGopher(p)
	}
	// Other people.
	others := map[string]string{
		"adonovan": "adonovan@google.com",
		"brainman": "alex.brainman@gmail.com",
		"ality":    "ality@pbrane.org",
		"dfc":      "dave@cheney.net",
		"dvyukov":  "dvyukov@google.com",
		"gustavo":  "gustavo@niemeyer.net",
		"jsing":    "jsing@google.com",
		"mikio":    "mikioh.mikioh@gmail.com",
		"minux":    "minux.ma@gmail.com",
		"remy":     "remyoudompheng@gmail.com",
		"rminnich": "rminnich@gmail.com",
	}
	for p, e := range others {
		personList = append(personList, p)
		emailToPerson[e] = p
		preferredEmail[p] = e
	}
	for _, p := range victims {
		if e := preferredEmail[p]; e == "" {
			addGopher(p)
		}
	}

	sort.Strings(personList)
}
