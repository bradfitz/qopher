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
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	dir    = flag.String("dir", "", "Directory to put data files")
	update = flag.Duration("update", 0, "If non-zero, duratation in past to update from.")
	report = flag.Bool("report", false, "Dump a report")
)

const (
	maxItemsPerPage = 1000

	rvTimeFormat = "2006-01-02 15:04:05"

	// closed=1 means "unknown"
	queryTmpl = "https://codereview.appspot.com/search?closed=1&owner=&{{CC_OR_REVIEWER}}=golang-dev@googlegroups.com&repo_guid=&base=&private=1&created_before=&created_after=&modified_before=&modified_after=&order=-modified&format=json&keys_only=False&with_messages=False&cursor={{CURSOR}}&limit={{LIMIT}}"

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

	if *update != 0 {
		for _, to := range []string{"reviewer", "cc"} {
			updatewg.Add(1)
			go loadReviews(to, updatewg)
		}
		updatewg.Wait()

		for _, r := range allReviews() {
			for _, patchID := range r.PatchSets {
				updatewg.Add(1)
				go func(r *Review, id int) {
					defer updatewg.Done()
					if err := r.LoadPatchMeta(id); err != nil {
						log.Fatal(err)
					}
				}(r, patchID)
			}
		}
		updatewg.Wait()
	}

	if *report {
		open := 0
		closed := 0
		for _, r := range allReviews() {
			if r.Closed {
				closed++
			} else {
				open++
			}
		}
		fmt.Printf("open: %d, closed: %d\n", open, closed)
	}
}

func allReviews() []*Review {
	var ret []*Review
	issueRE := regexp.MustCompile(`^([0-9]+)\.json$`)
	err := filepath.Walk(filepath.Join(*dir, "reviews"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if m := issueRE.FindStringSubmatch(info.Name()); m != nil {
			id, _ := strconv.Atoi(m[1])
			r, err := loadDiskFullReview(id)
			if err != nil {
				return err
			}
			ret = append(ret, r)
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Error loading all issues from disk: %v", err)
	}
	return ret
}

func loadReviews(to string, wg *sync.WaitGroup) {
	defer wg.Done()
	cursor := ""
	oldestWant := time.Now().Add(-*update).Format(rvTimeFormat)
	for {
		url := urlWithParams(queryTmpl, map[string]string{
			"CC_OR_REVIEWER": to,
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
		ngood := 0 // where good means "new enough"
		nold := 0
		for _, r := range reviews {
			if r.Modified >= oldestWant {
				ngood++
				wg.Add(1)
				go updateReview(r, wg)
			} else {
				nold++
			}
		}
		log.Printf("for cursor %q, Got %d reviews (%d in time window, %d old)", cursor, len(reviews), ngood, nold)
		res.Body.Close()
		if cursor == "" || ngood == 0 || nold > 0 {
			break
		}
	}
}

var httpGate = make(chan bool, 25)

func gate() (ungate func()) {
	httpGate <- true
	return func() {
		<-httpGate
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

	dstFile := issueDiskPath(r.Issue)
	dir := filepath.Dir(dstFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatal(err)
	}

	defer gate()()

	url := urlWithParams(reviewTmpl, map[string]string{
		"ISSUE_NUMBER": fmt.Sprint(r.Issue),
	})
	log.Printf("Fetching %s", url)
	res, err := http.Get(url)
	if err != nil || res.StatusCode != 200 {
		log.Fatalf("Error fetching %s: %+v, %v", url, res, err)
	}
	defer res.Body.Close()

	if err := writeReadableJSON(dstFile, res.Body); err != nil {
		log.Fatal(err)
	}
}

func writeReadableJSON(dstFile string, r io.Reader) error {
	all, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, all, "", "\t"); err != nil {
		log.Fatal(err)
	}

	dir := filepath.Dir(dstFile)
	tf, err := ioutil.TempFile(dir, "tmp")
	if err != nil {
		return err
	}
	_, err = tf.Write(buf.Bytes())
	if err != nil {
		return err
	}
	if err := tf.Close(); err != nil {
		return err
	}
	return os.Rename(tf.Name(), dstFile)
}

func issueDiskPath(id int) string {
	base := fmt.Sprintf("%d.json", id)
	return filepath.Join(*dir, "reviews", base[:3], base)
}

func patchDiskPatch(issue, patch int) string {
	base := fmt.Sprintf("%d-patch-%d.json", issue, patch)
	return filepath.Join(*dir, "reviews", base[:3], base)
}

func loadDiskFullReview(id int) (*Review, error) {
	path := issueDiskPath(id)
	f, err := os.Open(path)
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
	Closed     bool       `json:"closed"`
	PatchSets  []int      `json:"patchsets"`
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

func (r *Review) LoadPatchMeta(patch int) error {
	path := patchDiskPatch(r.Issue, patch)
	if fi, err := os.Stat(path); err == nil && fi.Size() != 0 {
		return nil
	}

	defer gate()()

	url := fmt.Sprintf("https://codereview.appspot.com/api/%d/%d", r.Issue, patch)
	log.Printf("Fetching patch %s", url)
	res, err := http.Get(url)
	if err != nil || res.StatusCode != 200 {
		return fmt.Errorf("Error fetching %s (issue %d, patch %d): %+v, %v", url, r.Issue, patch, res, err)
	}
	defer res.Body.Close()

	return writeReadableJSON(path, res.Body)
}
