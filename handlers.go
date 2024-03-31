package webfinger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"git.sr.ht/~mariusor/lw"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
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

type filterObjectFn func(vocab.Object) bool
type filterActorFn func(vocab.Actor) bool

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
	if vocab.IsNil(it) {
		return false
	}
	if vocab.IsIRI(it) || vocab.IsObject(it) {
		return iri.Equals(it.GetLink(), false)
	}

	match := false
	if vocab.IsItemCollection(it) {
		_ = vocab.OnCollectionIntf(it, func(col vocab.CollectionInterface) error {
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

func CheckActorName(name string) filterActorFn {
	return func(a vocab.Actor) bool {
		return ValueMatchesLangRefs(vocab.Content(name), a.PreferredUsername, a.Name)
	}
}

func CheckObjectURL(url string) filterObjectFn {
	return func(a vocab.Object) bool {
		return iriMatchesItem(vocab.IRI(url), a.URL)
	}
}

func CheckActorURL(url string) filterActorFn {
	return func(a vocab.Actor) bool {
		return iriMatchesItem(vocab.IRI(url), a.URL)
	}
}

func CheckObjectID(url string) filterObjectFn {
	return func(o vocab.Object) bool {
		return iriMatchesItem(vocab.IRI(url), o.ID)
	}
}

func CheckActorID(url string) filterActorFn {
	return func(a vocab.Actor) bool {
		return iriMatchesItem(vocab.IRI(url), a.ID)
	}
}

func LoadIRI(dbs []Storage, what vocab.IRI, checkFns ...filterObjectFn) (vocab.Item, error) {
	var found vocab.Item

	for _, db := range dbs {
		result, err := db.Load(what)
		if err != nil {
			continue
		}
		err = vocab.OnObject(result, func(o *vocab.Object) error {
			for _, fn := range checkFns {
				if fn(*o) {
					found = o
				}
			}
			return nil
		})
	}
	if !vocab.IsNil(found) {
		return found, nil
	}
	return LoadActor(dbs, convertToActorChecks(checkFns...)...)
}

func convertToActorChecks(checkFns ...filterObjectFn) []filterActorFn {
	actorCheckFns := make([]filterActorFn, 0, len(checkFns))
	for _, fn := range checkFns {
		actorCheckFns = append(actorCheckFns, func(actor vocab.Actor) bool {
			result := false
			_ = vocab.OnObject(actor, func(ob *vocab.Object) error {
				result = fn(*ob)
				return nil
			})
			return result
		})
	}
	return actorCheckFns
}

func LoadActor(dbs []Storage, checkFns ...filterActorFn) (vocab.Item, error) {
	var found vocab.Item
	var err error

	for _, db := range dbs {
		inCollection := actors.IRI(db.Root.GetLink())
		actors, err := db.Load(inCollection)
		if err != nil {
			return nil, errors.NewNotFound(err, "no actors found in collection: %s", inCollection)
		}
		if actors.IsCollection() {
			err = vocab.OnCollectionIntf(actors, func(col vocab.CollectionInterface) error {
				for _, act := range col.Collection() {
					vocab.OnActor(act, func(a *vocab.Actor) error {
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
		} else {
			err = vocab.OnActor(actors, func(a *vocab.Actor) error {
				for _, fn := range checkFns {
					if fn(*a) {
						found = a
					}
				}
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
		a, err := LoadActor(h.s, CheckActorName(handle), CheckActorURL(handle), CheckActorID(handle))
		if err != nil {
			handleErr(h.l)(r, errors.NewNotFound(err, "resource not found %s", res)).ServeHTTP(w, r)
			return
		}
		if a == nil {
			handleErr(h.l)(r, errors.NotFoundf("resource not found %s", res)).ServeHTTP(w, r)
			return
		}
		result = a
	}
	if typ == "https" {
		ob, err := LoadIRI(h.s, vocab.IRI(res), CheckObjectURL(res), CheckObjectID(res))
		if err != nil {
			handleErr(h.l)(r, errors.NewNotFound(err, "resource not found %s", res)).ServeHTTP(w, r)
			return
		}
		if ob == nil {
			handleErr(h.l)(r, errors.NotFoundf("resource not found %s", res)).ServeHTTP(w, r)
			return
		}
		result = ob
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
	vocab.OnObject(result, func(ob *vocab.Object) error {
		if vocab.IsNil(ob.URL) {
			return nil
		}
		urls := make(vocab.IRIs, 0)
		if vocab.IsItemCollection(ob.URL) {
			vocab.OnItemCollection(ob.URL, func(col *vocab.ItemCollection) error {
				for _, it := range col.Collection() {
					urls.Append(it.GetLink())
				}
				return nil
			})
		} else {
			urls.Append(ob.URL.GetLink())
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
