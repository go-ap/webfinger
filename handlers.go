package webfinger

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"git.sr.ht/~mariusor/lw"
	"git.sr.ht/~mariusor/storage-all"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/auth"
	"github.com/go-ap/errors"
	"github.com/go-ap/filters"
	"github.com/writeas/go-nodeinfo"
)

type handler struct {
	s []Storage
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
	for _, db := range dbs {
		if filters.Any(checkFns...).Match(db.Root) {
			return db.Root, nil
		}

		serviceIRI := db.Root.GetLink()
		all, _ := db.Load(actors.IRI(db.Root))
		if vocab.IsNil(all) {
			continue
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
	return nil, errors.NotFoundf("no actors found in storage")
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
		a, err := LoadActor(h.s, FilterName(handle))
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

type NodeInfoResolver struct {
	users    int
	comments int
	posts    int
}

var (
	ValidActorTypes = vocab.ActivityVocabularyTypes{
		vocab.PersonType,
		vocab.ServiceType,
		vocab.GroupType,
		vocab.ApplicationType,
		vocab.OrganizationType,
	}
	ValidContentTypes = vocab.ActivityVocabularyTypes{
		vocab.ArticleType,
		vocab.NoteType,
		vocab.LinkType,
		vocab.PageType,
		vocab.DocumentType,
		vocab.VideoType,
		vocab.AudioType,
	}

	actorsFilter = filters.Object(filters.HasType(ValidActorTypes...))
	postsFilter  = filters.Object(filters.NilInReplyTo, filters.HasType(ValidContentTypes...))
	allFilter    = filters.Object(filters.HasType(ValidContentTypes...))
)

func NodeInfoResolverNew(r storage.ReadStore, app vocab.Actor) NodeInfoResolver {
	n := NodeInfoResolver{}
	if r == nil {
		return n
	}

	inboxOf := vocab.Inbox.Of(app)
	if vocab.IsNil(inboxOf) {
		return n
	}

	ff := filters.Checks{
		filters.HasType(vocab.CreateType),
		filters.Object(filters.IDLike(string(app.ID))),
	}
	col, err := r.Load(inboxOf.GetLink(), ff...)
	if err != nil {
		return n
	}
	var allItems vocab.ItemCollection
	_ = vocab.OnCollectionIntf(col, func(col vocab.CollectionInterface) error {
		allItems = col.Collection()
		return nil
	})
	_ = vocab.OnCollectionIntf(filters.Checks{actorsFilter}.Run(allItems), func(col vocab.CollectionInterface) error {
		n.users = len(col.Collection())
		return nil
	})
	_ = vocab.OnCollectionIntf(filters.Checks{postsFilter}.Run(allItems), func(col vocab.CollectionInterface) error {
		n.posts = len(col.Collection())
		return nil
	})
	_ = vocab.OnCollectionIntf(filters.Checks{allFilter}.Run(allItems), func(col vocab.CollectionInterface) error {
		n.comments = len(col.Collection())
		return nil
	})

	n.comments -= n.posts
	return n
}

func (n NodeInfoResolver) IsOpenRegistration() (bool, error) {
	// TODO(marius)
	return true, nil
}

func (n NodeInfoResolver) Usage() (nodeinfo.Usage, error) {
	u := nodeinfo.Usage{
		Users: nodeinfo.UsageUsers{
			Total: n.users,
		},
		LocalComments: n.comments,
		LocalPosts:    n.posts,
	}
	return u, nil
}

const (
	softwareName = "FedBOX"
	sourceURL    = "https://git.sr.ht/~mariusor/fedbox"
)

var Version = "HEAD"

func NodeInfoConfig(app vocab.Actor, ni WebInfo) nodeinfo.Config {
	var baseURL string
	if !vocab.IsNil(app.URL) {
		baseURL = string(app.URL.GetLink())
	}
	var attributedToURL string
	if !vocab.IsNil(app.AttributedTo) {
		attributedToURL = string(app.AttributedTo.GetLink())
	}
	return nodeinfo.Config{
		BaseURL: baseURL,
		InfoURL: "/nodeinfo",

		Metadata: nodeinfo.Metadata{
			NodeName:        string(regexp.MustCompile(`<[/\w]+>`).ReplaceAll([]byte(ni.Title), []byte{})),
			NodeDescription: ni.Summary,
			Private:         false,
			Software: nodeinfo.SoftwareMeta{
				GitHub:   sourceURL,
				HomePage: baseURL,
				Follow:   attributedToURL,
			},
		},
		Protocols: []nodeinfo.NodeProtocol{
			nodeinfo.ProtocolActivityPub,
		},
		Services: nodeinfo.Services{
			Inbound:  []nodeinfo.NodeService{},
			Outbound: []nodeinfo.NodeService{},
		},
		Software: nodeinfo.SoftwareInfo{
			Name:    path.Base(softwareName),
			Version: Version,
		},
	}
}

type WebInfo struct {
	Title       string   `json:"title"`
	Email       string   `json:"email"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Thumbnail   string   `json:"thumbnail,omitempty"`
	Languages   []string `json:"languages"`
	URI         string   `json:"uri"`
	Urls        []string `json:"urls,omitempty"`
	Version     string   `json:"version"`
}

type aggRepo []Storage

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

func (a aggRepo) Load(iri vocab.IRI, ff ...filters.Check) (vocab.Item, error) {
	var repo storage.ReadStore
	for _, db := range a {
		if db.Root.ID.Equal(baseIRI(iri)) {
			repo = db
		}
	}
	if repo == nil {
		return nil, errors.NotFoundf("unable to find item in any of the storage options")
	}
	return repo.Load(iri, ff...)
}

func reqBaseIRI(r http.Request, secure bool) vocab.IRI {
	scheme := "http"
	if secure || r.TLS != nil {
		scheme = "https"
	}
	u := url.URL{
		Scheme: scheme,
		Host:   r.Host,
	}
	u.Scheme = scheme
	u.Host = r.Host
	return vocab.IRI(u.String())
}

func findApp(s []Storage, rootIRI vocab.IRI) (*vocab.Actor, error) {
	var app *vocab.Actor
	maybeActor, _ := LoadActor(s, FilterURL(rootIRI.String()))
	if vocab.IsNil(maybeActor) {
		return nil, errors.NotFoundf("unable to find root application %s", rootIRI)
	}
	if actor, _ := vocab.ToActor(maybeActor); !vocab.IsNil(actor) {
		if vocab.ApplicationType.Match(actor.Type) && !vocab.IsNil(actor.AttributedTo) {
			app, _ = findApp(s, actor.AttributedTo.GetLink())
		} else {
			app = actor
		}
	}
	var err error
	if vocab.IsNil(app) {
		err = errors.NotFoundf("unable to find root application %s", rootIRI)
	}
	return app, err
}

func IconOf(it vocab.Item) string {
	var iconURL string
	if vocab.IsObject(it) {
		_ = vocab.OnObject(it, func(ob *vocab.Object) error {
			if ob.Icon != nil {
				iconURL = string(ob.Icon.GetLink())
			}
			return nil
		})
	}
	return iconURL
}
func setupNodeInfo(r *http.Request, s []Storage) (*nodeinfo.Service, error) {
	var app vocab.Actor
	rootIRI := reqBaseIRI(*r, true)
	maybeActor, _ := findApp(s, rootIRI)
	if !vocab.IsNil(maybeActor) {
		app = *maybeActor
	}
	if app.ID == "" || auth.AnonymousActor.Equals(app) {
		return nil, errors.NotFoundf("root actor not found")
	}

	name := vocab.NameOf(app)
	if name == "" {
		name = vocab.PreferredNameOf(app)
	}
	cfg := NodeInfoConfig(app, WebInfo{
		Title:       name,
		Email:       "",
		Summary:     vocab.SummaryOf(app),
		Description: vocab.ContentOf(app),
		Thumbnail:   IconOf(app),
		URI:         string(app.ID),
		Urls:        nil,
		Version:     Version,
	})
	return nodeinfo.NewService(cfg, NodeInfoResolverNew(aggRepo(s), app)), nil
}

const NodeInfoDiscoverPath = "/.well-known/nodeinfo"

// NodeInfoDiscover handles "/.well-known/nodeinfo"
func (h handler) NodeInfoDiscover(w http.ResponseWriter, r *http.Request) {
	ni, err := setupNodeInfo(r, h.s)
	if err != nil {
		handleErr(h.l)(r, err).ServeHTTP(w, r)
		return
	}

	ni.NodeInfoDiscover(w, r)
}

const NodeInfoPath = "/nodeinfo"

// NodeInfo handles "/nodeinfo"
func (h handler) NodeInfo(w http.ResponseWriter, r *http.Request) {
	ni, err := setupNodeInfo(r, h.s)
	if err != nil {
		handleErr(h.l)(r, err).ServeHTTP(w, r)
		return
	}

	ni.NodeInfo(w, r)
}
