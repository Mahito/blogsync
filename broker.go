package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/motemen/go-wsse"
	"github.com/x-motemen/blogsync/atom"
)

type broker struct {
	*atom.Client
	*blogConfig
}

func newBroker(bc *blogConfig) *broker {
	return &broker{
		Client: &atom.Client{
			Client: &http.Client{
				Transport: &wsse.Transport{
					Username: bc.Username,
					Password: bc.Password,
				},
			},
		},
		blogConfig: bc,
	}
}

func (b *broker) FetchRemoteEntries() ([]*entry, error) {
	entries := []*entry{}
	fixedPageURL := fixedPageEndpointURL(b.blogConfig)
	urls := []string{
		entryEndPointUrl(b.blogConfig),
		fixedPageURL,
	}
	for url := ""; true; {
		if url == "" {
			if len(urls) == 0 {
				break
			}
			url, urls = urls[0], urls[1:]
		}

		feed, err := b.Client.GetFeed(url)
		if err != nil {
			if url == fixedPageURL {
				// Ignore errors in the case of fixed pages, because fixed page is the feature
				// only for pro users.
				break
			}
			return nil, err
		}

		for _, ae := range feed.Entries {
			e, err := entryFromAtom(&ae)
			if err != nil {
				return nil, err
			}
			entries = append(entries, e)
		}

		nextLink := feed.Links.Find("next")
		if nextLink != nil {
			url = nextLink.Href
		} else {
			url = ""
		}
	}

	return entries, nil
}

const entryExt = ".md" // TODO regard re.ContentType

func (b *broker) LocalPath(e *entry) string {
	return filepath.Join(b.localRoot(), e.URL.Path+entryExt)
}

func (b *broker) StoreFresh(e *entry, path string) (bool, error) {
	var localLastModified time.Time
	if fi, err := os.Stat(path); err == nil {
		localLastModified = fi.ModTime()
	}

	if e.LastModified.After(localLastModified) {
		logf("fresh", "remote=%s > local=%s", e.LastModified, localLastModified)
		if err := b.Store(e, path, ""); err != nil {
			return false, err
		}

		return true, nil
	}

	return false, nil
}

func (b *broker) Store(e *entry, path, origPath string) error {
	logf("store", "%s", path)

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if err := os.WriteFile(path, []byte(e.fullContent()), 0666); err != nil {
		return err
	}

	if err := os.Chtimes(path, *e.LastModified, *e.LastModified); err != nil {
		return err
	}

	if origPath != "" && path != origPath {
		return os.Remove(origPath)
	}

	return nil
}

func (b *broker) UploadFresh(e *entry) (bool, error) {
	re, err := asEntry(b.Client.GetEntry(e.EditURL))
	if err != nil {
		return false, err
	}

	if e.LastModified.After(*re.LastModified) == false {
		return false, nil
	}

	return true, b.PutEntry(e)
}

func (b *broker) PutEntry(e *entry) error {
	newEntry, err := asEntry(b.Client.PutEntry(e.EditURL, e.atom()))
	if err != nil {
		return err
	}
	if e.CustomPath != "" {
		newEntry.CustomPath = e.CustomPath
	}
	return b.Store(newEntry, b.LocalPath(newEntry), b.originalPath(e))
}

func (b *broker) PostEntry(e *entry, isPage bool) error {
	var endPoint string
	if !isPage {
		endPoint = entryEndPointUrl(b.blogConfig)
	} else {
		endPoint = fixedPageEndpointURL(b.blogConfig)
	}
	newEntry, err := asEntry(b.Client.PostEntry(endPoint, e.atom()))
	if err != nil {
		return err
	}
	if e.CustomPath != "" {
		newEntry.CustomPath = e.CustomPath
	}

	return b.Store(newEntry, b.LocalPath(newEntry), "")
}

func (b *broker) originalPath(e *entry) string {
	if e.URL == nil {
		return ""
	}
	return b.LocalPath(e)
}

func atomEndpointURLRoot(bc *blogConfig) string {
	owner := bc.Owner
	if owner == "" {
		owner = bc.Username
	}
	return fmt.Sprintf("https://blog.hatena.ne.jp/%s/%s/atom/", owner, bc.BlogID)
}

func entryEndPointUrl(bc *blogConfig) string {
	return atomEndpointURLRoot(bc) + "entry"
}

func fixedPageEndpointURL(bc *blogConfig) string {
	return atomEndpointURLRoot(bc) + "page"
}
