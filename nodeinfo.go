package webfinger

import (
	"net/http"
	"net/url"
	"path"
	"regexp"

	"git.sr.ht/~mariusor/storage-all"
	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/auth"
	"github.com/go-ap/errors"
	"github.com/go-ap/filters"
	"github.com/writeas/go-nodeinfo"
)

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
	h.l.Debugf("%s %s%s %d %s", r.Method, r.Host, r.RequestURI, http.StatusOK, http.StatusText(http.StatusOK))
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
	h.l.Debugf("%s %s%s %d %s", r.Method, r.Host, r.RequestURI, http.StatusOK, http.StatusText(http.StatusOK))
}
