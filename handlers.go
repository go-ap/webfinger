package webfinger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-ap/processing"
)

type handler struct {
	s   processing.ReadStore
	app vocab.Item
}

func New(app vocab.Actor, db processing.ReadStore) handler {
	return handler{s: db, app: app}
}

var actors = vocab.CollectionPath("actors")

func ValueMatchesLangRefs(val vocab.Content, toCheck ...vocab.NaturalLanguageValues) bool {
	for _, lr := range toCheck {
		for _, name := range lr {
			if strings.EqualFold(name.String(), val.String()) {
				return true
			}
		}
	}
	return false
}
func iriMatchesItem(iri vocab.IRI, it vocab.Item) bool {
	if vocab.IsIRI(it) || vocab.IsObject(it) {
		return iri.Equals(it.GetLink(), false)
	}

	match := false
	if vocab.IsItemCollection(it) {
		vocab.OnCollectionIntf(it, func(col vocab.CollectionInterface) error {
			for _, i := range col.Collection() {
				if iri.Equals(i.GetLink(), true) {
					match = true
					break
				}
			}
			return nil
		})
	}
	return match
}

func CheckActorName(name string) func(actor vocab.Actor) bool {
	return func(a vocab.Actor) bool {
		return ValueMatchesLangRefs(vocab.Content(name), a.PreferredUsername, a.Name)
	}
}

func CheckActorURL(url string) func (actor vocab.Actor) bool {
	return func(a vocab.Actor) bool {
		return iriMatchesItem(vocab.IRI(url), a.URL)
	}
}

func LoadActor(db processing.ReadStore, inCollection vocab.IRI, checkFns ...func(actor vocab.Actor) bool) (*vocab.Actor, error) {
	actors, err := db.Load(inCollection)
	if err != nil {
		return nil, errors.NewNotFound(err, "no actors found in collection: %s", inCollection)
	}
	var found *vocab.Actor
	err = vocab.OnCollectionIntf(actors, func(col vocab.CollectionInterface) error {
		for _, actor := range col.Collection() {
			vocab.OnActor(actor, func(a *vocab.Actor) error {
				for _, fn := range checkFns {
					if fn(*a) {
						found = a
					}
				}
				return nil
			})
		}
		return nil
	})
	return found, err
}

// HandleWebFinger serves /.well-known/webfinger/
func (h handler) HandleWebFinger(w http.ResponseWriter, r *http.Request) {
	res := r.URL.Query().Get("resource")

	var host string

	typ, handle := splitResourceString(res)
	if typ == "" || handle == "" {
		errors.HandleError(errors.BadRequestf("invalid resource %s", res)).ServeHTTP(w, r)
		return
	}
	if typ == "https" {
		if u, err := url.ParseRequestURI(res); err == nil {
			handle = res
			host = u.Host
		}
	} else {
		if strings.Contains(handle, "@") {
			handle, host = func(s string) (string, string) {
				split := "@"
				ar := strings.Split(s, split)
				if len(ar) != 2 {
					return "", ""
				}
				return ar[0], ar[1]
			}(handle)
		} else {
			host = r.Host
		}
	}

	wf := node{}
	subject := fmt.Sprintf("%s@%s", handle, host)

	actorsIRI := actors.IRI(h.app.GetLink())
	a, err := LoadActor(h.s, actorsIRI, CheckActorName(handle), CheckActorURL(handle))
	if err != nil {
		errors.HandleError(errors.NewNotFound(err, "resource not found %s", res)).ServeHTTP(w, r)
		return
	}
	if a == nil {
		errors.HandleError(errors.NotFoundf("resource not found %s", res)).ServeHTTP(w, r)
		return
	}

	id := a.GetID()
	wf.Subject = subject
	wf.Links = []link{
		{
			Rel:  "self",
			Type: "application/activity+json",
			Href: id.String(),
		},
	}
	urls := make(vocab.ItemCollection, 0)
	if vocab.IsItemCollection(a.URL) {
		urls = append(urls, a.URL.(vocab.ItemCollection)...)
	} else {
		urls = append(urls, a.URL.(vocab.IRI))
	}

	for _, u := range urls {
		url := u.GetLink().String()
		wf.Aliases = append(wf.Aliases, url)
		wf.Links = append(wf.Links, link{
			Rel:  "https://webfinger.net/rel/profile-page",
			Type: "text/html",
			Href: url,
		})
	}
	wf.Aliases = append(wf.Aliases, id.String())

	dat, _ := json.Marshal(wf)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}

// HandleHostMeta serves /.well-known/host-meta
func (h handler) HandleHostMeta(w http.ResponseWriter, r *http.Request) {
	hm := node{
		Subject: "",
		Aliases: nil,
		Links: []link{
			{
				Rel:      "lrdd",
				Type:     "application/xrd+json",
				Template: fmt.Sprintf("https://%s/.well-known/node?resource={uri}", r.Host),
			},
		},
	}
	dat, _ := json.Marshal(hm)

	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
}
