package webfinger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"git.sr.ht/~mariusor/lw"
	"git.sr.ht/~mariusor/storage-all"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/errors"
	"github.com/go-ap/filters"
)

type handler struct {
	s []Store
	l lw.Logger
}

type Store interface {
	Open() error
	Close()
	storage.ReadStore
}

type Storage struct {
	Store
	Root vocab.Actor
}

func New(l lw.Logger, db ...Store) handler {
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

func LoadIRI(db Storage, what vocab.IRI, checkFns ...filters.Check) (vocab.Item, error) {
	var found vocab.Item

	serviceIRI := db.Root.GetLink()
	result, err := db.Load(what, append(checkFns, filters.Authorized(serviceIRI))...)
	if err != nil {
		return nil, errors.NewNotFound(err, "no actors found in storage")
	}
	err = vocab.OnObject(result, func(o *vocab.Object) error {
		found = o
		return nil
	})
	if err != nil {
		return nil, errors.NewNotFound(err, "no actors found in storage")
	}
	if !vocab.IsNil(found) {
		return found, nil
	}
	return LoadActor(db, checkFns...)
}

func LoadActor(db Storage, checkFns ...filters.Check) (vocab.Item, error) {
	if filters.Any(checkFns...).Match(db.Root) {
		return db.Root, nil
	}

	serviceIRI := db.Root.GetLink()
	all, _ := db.Load(actors.IRI(db.Root))
	if vocab.IsNil(all) {
		return nil, errors.NotFoundf("no actors found in storage")
	}

	checkFns = append(checkFns, filters.Authorized(serviceIRI))
	if all.IsCollection() {
		all = filters.Checks(checkFns).Run(all)
		err := vocab.OnCollectionIntf(all, func(col vocab.CollectionInterface) error {
			all = col.Collection().First()
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return vocab.ToActor(all)
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

const WellKnownWebFingerPath = "/.well-known/webfinger"

func baseURL(r *http.Request) []string {
	if r == nil {
		return nil
	}

	// NOTE(marius): due to the fact that the Authorize server runs behind a proxy which handles the TLS termination,
	// we can't rely on the request's TLS property to determine the scheme for our URL,
	// so we generate two base URLs, one for each scheme.
	return []string{
		fmt.Sprintf("http://%s", r.Host),
		fmt.Sprintf("https://%s", r.Host),
	}
}

var errStorageNotFound = errors.NotFoundf("matching storage not found")

func (h *handler) findMatchingStorage(hosts ...string) (Storage, error) {
	var app vocab.Actor
	for _, db := range h.s {
		for _, host := range hosts {
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
				return Storage{Root: app, Store: db}, nil
			}
		}
	}
	return Storage{Root: app, Store: nil}, errStorageNotFound
}

// HandleWebFinger serves /.well-known/webfinger/
func (h handler) HandleWebFinger(w http.ResponseWriter, r *http.Request) {
	storage, err := h.findMatchingStorage(baseURL(r)...)
	if err != nil {
		handleErr(h.l)(r, err).ServeHTTP(w, r)
		return
	}

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
		a, err := LoadActor(storage, FilterName(handle))
		if err != nil {
			handleErr(h.l)(r, errors.NewNotFound(err, "resource not found %s", res)).ServeHTTP(w, r)
			return
		}
		result = a
	}
	if typ == "https" {
		ob, err := LoadIRI(storage, vocab.IRI(res), filters.Any(FilterURL(res), FilterID(res)))
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
			us := u.String()
			wf.Aliases = append(wf.Aliases, us)
			wf.Links = append(wf.Links, link{
				Rel:  "https://webfinger.net/rel/profile-page",
				Type: "text/html",
				Href: us,
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

const WellKnownHostPath = "/.well-known/host-meta"

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

func baseIRI(i vocab.IRI) vocab.IRI {
	u, _ := i.URL()
	if u == nil {
		return i
	}
	u.Path = ""
	u.RawFragment = ""
	u.RawQuery = ""
	return vocab.IRI(u.String())
}

type aggRepo Storage

func (a aggRepo) Load(iri vocab.IRI, ff ...filters.Check) (vocab.Item, error) {
	var repo storage.ReadStore
	db := Storage(a)
	if db.Root.ID.Equal(baseIRI(iri)) {
		repo = db
	}
	if repo == nil {
		return nil, errors.NotFoundf("unable to find item in any of the storage options")
	}
	return repo.Load(iri, ff...)
}
