// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	dir   = flag.String("dir", "", "Directory to put data files")
	since = flag.Duration("since", 1*time.Hour, "Fetch things modified in this past duration.")
)

const (
	maxItemsPerPage = 1000

	rvTimeFormat = "2006-01-02 15:04:05"

	queryTmpl = "https://codereview.appspot.com/search?closed=3&{{CC_OR_REVIEWER}}=golang-dev%40googlegroups.com&private=1&modified_before={{DATE_BEFORE}}&modifed_after={{DATE_AFTER}}&order=-modified&keys_only=False&with_messages=False&cursor={{CURSOR}}&limit={{LIMIT}}&format=json"

	// JSON with the text of messages. e.g.
	// https://codereview.appspot.com/api/6454085?messages=true
	reviewTmpl = "https://codereview.appspot.com/api/{{ISSUE_NUMBER}}?messages=true"
)

// itemsPerPage is the number of items to fetch for a single page.
// Changed by tests.
var itemsPerPage = 100 // maxItemsPerPage

var updatewg = new(sync.WaitGroup)

func main() {
	flag.Parse()
	if *dir == "" {
		log.Fatalf("--dir flag is required.")
	}
	if fi, err := os.Stat(*dir); err != nil || !fi.IsDir() {
		log.Fatalf("Directory %q needs to exist.", *dir)
	}

	for _, to := range []string{"reviewer", "cc"} {
		updatewg.Add(1)
		go loadReviews(to, updatewg)
	}
	updatewg.Wait()
}

func loadReviews(to string, wg *sync.WaitGroup) {
	defer wg.Done()
	cursor := ""
	for {
		url := urlWithParams(queryTmpl, map[string]string{
			"CC_OR_REVIEWER": to,
			"DATE_AFTER":     time.Now().Add(-*since).Format(rvTimeFormat),
			"DATE_BEFORE":    "2020-01-01 00:00:00",
			"CURSOR":         cursor,
			"LIMIT":          fmt.Sprint(itemsPerPage),
		})
		log.Printf("Fetching %s", url)
		res, err := http.Get(url)
		if err != nil {
			log.Fatal(err)
		}
		var reviews []*Review
		reviews, cursor, err = ParseReviews(res.Body)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("for cursor %q, Got %d reviews", cursor, len(reviews))
		for _, r := range reviews {
			wg.Add(1)
			go updateReview(r, wg)
		}
		res.Body.Close()
		if cursor == "" || len(reviews) == 0 {
			break
		}
	}
}

// updateReview checks to see if r (which lacks comments) has a higher
// modification time than the version we have on disk and if necessary
// fetches the full (with comments) version of r and puts it on disk.
func updateReview(r *Review, wg *sync.WaitGroup) {
	defer wg.Done()
	dr, err := loadDiskFullReview(r.Issue)
	if err == nil && dr.Modified == r.Modified {
		// Nothing to do.
		return
	}
	if err != nil && !os.IsNotExist(err) {
		log.Fatalf("Error loading issue %d: %v", r.Issue, err)
	}
	url := urlWithParams(reviewTmpl, map[string]string{
		"ISSUE_NUMBER": fmt.Sprint(r.Issue),
	})
	log.Printf("Fetching %s", url)
	res, err := http.Get(url)
	if err != nil || res.StatusCode != 200 {
		log.Fatalf("Error fetching %s: %+v, %v", url, res, err)
	}
	defer res.Body.Close()
	all, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatal(err)
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, all, "", "\t"); err != nil {
		log.Fatal(err)
	}

	dstFile := issueDiskPath(r.Issue)
	dir := filepath.Dir(dstFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatal(err)
	}
	tf, err := ioutil.TempFile(dir, "issue")
	if err != nil {
		log.Fatal(err)
	}
	_, err = tf.Write(buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}
	if err := tf.Close(); err != nil {
		log.Fatal(err)
	}
	if err := os.Rename(tf.Name(), dstFile); err != nil {
		log.Fatal(err)
	}
}

func issueDiskPath(id int) string {
	return filepath.Join(*dir, "reviews", fmt.Sprintf("%d.json", id))
}

func loadDiskFullReview(id int) (*Review, error) {
	f, err := os.Open(issueDiskPath(id))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := new(Review)
	return r, json.NewDecoder(f).Decode(&r)
}

var urlParam = regexp.MustCompile(`{{\w+}}`)

func urlWithParams(urlTempl string, m map[string]string) string {
	return urlParam.ReplaceAllStringFunc(urlTempl, func(param string) string {
		return url.QueryEscape(m[strings.Trim(param, "{}")])
	})
}

type issueResult struct {
	Closed   bool   `json:"closed"`
	Modified string `json:"modified"`
}

type Message struct {
	Sender string `json:"sender"`
	Text   string `json:"text"`
	Date   string `json:"date"` // "2012-04-07 00:51:58.602055"
}

type byMessageDate []*Message

func (s byMessageDate) Len() int           { return len(s) }
func (s byMessageDate) Less(i, j int) bool { return s[i].Date < s[j].Date }
func (s byMessageDate) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

type monthQueryResult struct {
	Cursor  string    `json:"cursor"`
	Results []*Review `json:"results"`
}

// Time unmarshals a time in rietveld's format.
type Time time.Time

func (t *Time) UnmarshalJSON(b []byte) error {
	if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
		return fmt.Errorf("types: failed to unmarshal non-string value %q as an RFC 3339 time")
	}
	// TODO: pic
	tm, err := time.Parse("2006-01-02 15:04:05", string(b[1:len(b)-1]))
	if err != nil {
		return err
	}
	*t = Time(tm)
	return nil
}

func (t Time) String() string { return time.Time(t).String() }

type Review struct {
	Issue      int        `json:"issue"`
	Desc       string     `json:"description"`
	OwnerEmail string     `json:"owner_email"`
	Owner      string     `json:"owner"`
	Created    Time       `json:"created"`
	Modified   string     `json:"modified"` // just a string; more reliable to do string equality tests on it
	Messages   []*Message `json:"messages"`
	Reviewers  []string   `json:"reviewers"`
	CC         []string   `json:"cc"`
}

func ParseReviews(r io.Reader) (reviews []*Review, cursor string, err error) {
	var d monthQueryResult
	err = json.NewDecoder(r).Decode(&d)
	return d.Results, d.Cursor, err
}

// Reviewer returns the email address of an explicit reviewer, if any, else
// returns the empty string.
func (r *Review) Reviewer() string {
	for _, who := range r.Reviewers {
		if strings.HasSuffix(who, "@googlegroups.com") {
			continue
		}
		return who
	}
	return ""
}
