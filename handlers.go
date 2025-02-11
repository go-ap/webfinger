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
	"github.com/go-ap/filters"
	"github.com/go-ap/processing"
)

type handler struct {
	s []Storage
	l lw.Logger
}

type Storage struct {
	processing.ReadStore
	Root vocab.Actor
}

func New(l lw.Logger, db ...Storage) handler {
	return handler{s: db, l: l}
}

var actors = vocab.CollectionPath("actors")

func FilterName(name string) filters.Check {
	return filters.NameIs(name)
}

func FilterURL(u string) filters.Check {
	if _, err := url.ParseRequestURI(u); err != nil {
		u = "https://" + u
	}
	return filters.SameURL(vocab.IRI(u))
}

func FilterID(id string) filters.Check {
	return filters.SameID(vocab.ID(id))
}

func LoadIRI(dbs []Storage, what vocab.IRI, checkFns ...filters.Check) (vocab.Item, error) {
	var found vocab.Item

	for _, db := range dbs {
		serviceIRI := db.Root.GetLink()
		result, err := db.Load(what, append(checkFns, filters.Authorized(serviceIRI))...)
		if err != nil {
			continue
		}
		err = vocab.OnObject(result, func(o *vocab.Object) error {
			found = o
			return nil
		})
	}
	if !vocab.IsNil(found) {
		return found, nil
	}
	return LoadActor(dbs, checkFns...)
}

func LoadActor(dbs []Storage, checkFns ...filters.Check) (vocab.Item, error) {
	var found vocab.Item
	var err error

	for _, db := range dbs {
		if filters.Any(checkFns...).Match(db.Root) {
			return db.Root, nil
		}

		serviceIRI := db.Root.GetLink()
		inCollection := actors.IRI(serviceIRI)
		actors, err := db.Load(inCollection, append(checkFns, filters.Authorized(serviceIRI))...)
		if err != nil {
			return nil, errors.NewNotFound(err, "no actors found in collection: %s", inCollection)
		}
		if actors.IsCollection() {
			err = vocab.OnCollectionIntf(actors, func(col vocab.CollectionInterface) error {
				for _, act := range col.Collection() {
					_ = vocab.OnActor(act, func(a *vocab.Actor) error {
						found = a
						return nil
					})
				}
				return nil
			})
		} else {
			err = vocab.OnActor(actors, func(a *vocab.Actor) error {
				found = a
				return nil
			})
		}
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

// HandleWebFinger serves /.well-known/webfinger/
func (h handler) HandleWebFinger(w http.ResponseWriter, r *http.Request) {
	res := r.URL.Query().Get("resource")
	if res == "" {
		handleErr(h.l)(r, errors.NotFoundf("resource not found %s", res)).ServeHTTP(w, r)
		return
	}

	hosts := make([]string, 0)
	hosts = append(hosts, r.Host)
	typ, handle := splitResourceString(res)
	if typ == "" || handle == "" {
		handleErr(h.l)(r, errors.BadRequestf("invalid resource %s", res)).ServeHTTP(w, r)
		return
	}
	if typ == "acct" {
		if strings.Contains(handle, "@") {
			nh, hh := func(s string) (string, string) {
				if ar := strings.Split(s, "@"); len(ar) == 2 {
					return ar[0], ar[1]
				}
				return s, ""
			}(handle)
			hosts = append(hosts, hh)
			handle = nh
		}
	}

	wf := node{}
	subject := res

	var result vocab.Item
	if typ == "acct" {
		a, err := LoadActor(h.s, filters.Any(FilterName(handle), FilterURL(handle), FilterID(handle)))
		if err != nil {
			handleErr(h.l)(r, errors.NewNotFound(err, "resource not found %s", res)).ServeHTTP(w, r)
			return
		}
		result = a
	}
	if typ == "https" {
		ob, err := LoadIRI(h.s, vocab.IRI(res), filters.Any(FilterURL(res), FilterID(res)))
		if err != nil {
			handleErr(h.l)(r, errors.NewNotFound(err, "resource not found %s", res)).ServeHTTP(w, r)
			return
		}
		result = ob
	}
	if result == nil || vocab.IsNil(result) {
		handleErr(h.l)(r, errors.NotFoundf("resource not found %s", res)).ServeHTTP(w, r)
		return
	}

	id := result.GetID()
	wf.Subject = subject
	wf.Links = []link{
		{
			Rel:  "self",
			Type: "application/activity+json",
			Href: id.String(),
		},
	}
	_ = vocab.OnObject(result, func(ob *vocab.Object) error {
		if vocab.IsNil(ob.URL) {
			return nil
		}
		urls := make(vocab.IRIs, 0)
		if vocab.IsItemCollection(ob.URL) {
			_ = vocab.OnItemCollection(ob.URL, func(col *vocab.ItemCollection) error {
				for _, it := range col.Collection() {
					_ = urls.Append(it.GetLink())
				}
				return nil
			})
		} else {
			_ = urls.Append(ob.URL.GetLink())
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

		wf.Aliases = append(wf.Aliases, id.String())
		return nil
	})

	dat, _ := json.Marshal(wf)
	w.Header().Set("Content-Type", "application/jrd+json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(dat)
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
	_, _ = w.Write(dat)
	h.l.Debugf("%s %s%s %d %s", r.Method, r.Host, r.RequestURI, http.StatusOK, http.StatusText(http.StatusOK))
}
