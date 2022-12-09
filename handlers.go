package webfinger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-ap/processing"
)

type handler struct {
	s []processing.ReadStore
	l lw.Logger
}

func New(l lw.Logger, db ...processing.ReadStore) handler {
	return handler{s: db, l: l}
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

func CheckActorURL(url string) func(actor vocab.Actor) bool {
	return func(a vocab.Actor) bool {
		return iriMatchesItem(vocab.IRI(url), a.URL)
	}
}

func CheckActorID(url string) func(actor vocab.Actor) bool {
	return func(a vocab.Actor) bool {
		return iriMatchesItem(vocab.IRI(url), a.ID)
	}
}

func LoadActor(db processing.ReadStore, app vocab.Actor, checkFns ...func(actor vocab.Actor) bool) (*vocab.Actor, error) {
	inCollection := actors.IRI(app.GetLink())
	actors, err := db.Load(inCollection)
	if err != nil {
		return nil, errors.NewNotFound(err, "no actors found in collection: %s", inCollection)
	}
	var found *vocab.Actor
	err = vocab.OnCollectionIntf(actors, func(col vocab.CollectionInterface) error {
		col.Append(app)
		for _, actor := range col.Collection() {
			err = vocab.OnActor(actor, func(a *vocab.Actor) error {
				for _, fn := range checkFns {
					if fn(*a) {
						found = a
					}
				}
				return nil
			})
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return found, err
}

func handleErr(l lw.Logger) func(r *http.Request, e error) errors.ErrorHandlerFn {
	return func(r *http.Request, e error) errors.ErrorHandlerFn {
		defer func(r *http.Request, e error) {
			st := errors.HttpStatus(e)
			l.Warnf("%s %s%s %d %s", r.Method, r.Host, r.RequestURI, st, http.StatusText(st))
		}(r, e)
		return errors.HandleError(e)
	}
}

func (h handler) findMatchingStorage(hosts ...string) (vocab.Actor, processing.ReadStore, error) {
	var app vocab.Actor
	for _, db := range h.s {
		for _, host := range hosts {
			host = "https://" + host + "/"
			res, err := db.Load(vocab.IRI(host))
			if err != nil {
				continue
			}
			err = vocab.OnActor(res, func(actor *vocab.Actor) error {
				app = *actor
				return nil
			})
			if err != nil {
				continue
			}
			if app.ID != "" {
				return app, db, nil
			}
		}
	}
	return app, nil, fmt.Errorf("unable to find storage")
}

// HandleWebFinger serves /.well-known/webfinger/
func (h handler) HandleWebFinger(w http.ResponseWriter, r *http.Request) {
	res := r.URL.Query().Get("resource")
	if res == "" {
		handleErr(h.l)(r, errors.NotFoundf("resource not found %s", res)).ServeHTTP(w, r)
		return
	}

	host := r.Host
	typ, handle := splitResourceString(res)
	if typ == "" || handle == "" {
		handleErr(h.l)(r, errors.BadRequestf("invalid resource %s", res)).ServeHTTP(w, r)
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
		}
	}

	app, db, err := h.findMatchingStorage(host)
	if err != nil {
		handleErr(h.l)(r, errors.NewNotFound(err, "resource not found %s", res)).ServeHTTP(w, r)
		return
	}

	wf := node{}
	subject := res

	a, err := LoadActor(db, app, CheckActorName(handle), CheckActorURL(handle), CheckActorID(handle))
	if err != nil {
		handleErr(h.l)(r, errors.NewNotFound(err, "resource not found %s", res)).ServeHTTP(w, r)
		return
	}
	if a == nil {
		handleErr(h.l)(r, errors.NotFoundf("resource not found %s", res)).ServeHTTP(w, r)
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
	if !vocab.IsNil(a.URL) {
		urls := make(vocab.IRIs, 0)
		if vocab.IsItemCollection(a.URL) {
			vocab.OnItemCollection(a.URL, func(col *vocab.ItemCollection) error {
				for _, it := range col.Collection() {
					urls.Append(it.GetLink())
				}
				return nil
			})
		} else {
			urls.Append(a.URL.GetLink())
		}

		for _, u := range urls {
			if u.Equals(id, true) {
				continue
			}
			url := u.String()
			wf.Aliases = append(wf.Aliases, url)
			wf.Links = append(wf.Links, link{
				Rel:  "https://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: url,
			})
		}
	}

	wf.Aliases = append(wf.Aliases, id.String())

	dat, _ := json.Marshal(wf)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	w.Write(dat)
	h.l.Debugf("%s %s%s %d %s", r.Method, r.Host, r.RequestURI, http.StatusOK, http.StatusText(http.StatusOK))
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
	h.l.Debugf("%s %s%s %d %s", r.Method, r.Host, r.RequestURI, http.StatusOK, http.StatusText(http.StatusOK))
}
