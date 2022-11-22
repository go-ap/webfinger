// Package webfinger
package webfinger

import (
	"path"
	"regexp"
	"strings"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/processing"
	"github.com/mariusor/qstring"
	"github.com/writeas/go-nodeinfo"
)

type link struct {
	Rel      string `json:"rel,omitempty"`
	Type     string `json:"type,omitempty"`
	Href     string `json:"href,omitempty"`
	Template string `json:"template,omitempty"`
}

type node struct {
	Subject string   `json:"subject"`
	Aliases []string `json:"aliases"`
	Links   []link   `json:"links"`
}

type NodeInfoResolver struct {
	users    int
	comments int
	posts    int
}

type CompStr = qstring.ComparativeString
type CompStrs []CompStr

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

	nilFilter  = EqualsString("-")
	nilFilters = CompStrs{nilFilter}

	actorsFilter = &Filters{
		Type: ActivityTypesFilter(ValidActorTypes...),
	}
	postsFilter = &Filters{
		Type: ActivityTypesFilter(ValidContentTypes...),
		OP:   nilFilters,
	}
	allFilter = &Filters{
		Type: ActivityTypesFilter(ValidContentTypes...),
	}
)

type Filters struct {
	Name       CompStrs `qstring:"name,omitempty"`
	Cont       CompStrs `qstring:"content,omitempty"`
	MedTypes   CompStrs `qstring:"mediaType,omitempty"`
	URL        CompStrs `qstring:"url,omitempty"`
	IRI        CompStrs `qstring:"iri,omitempty"`
	Generator  CompStrs `qstring:"generator,omitempty"`
	Type       CompStrs `qstring:"type,omitempty"`
	AttrTo     CompStrs `qstring:"attributedTo,omitempty"`
	InReplTo   CompStrs `qstring:"inReplyTo,omitempty"`
	OP         CompStrs `qstring:"context,omitempty"`
	Recipients CompStrs `qstring:"recipients,omitempty"`
	Next       string   `qstring:"after,omitempty"`
	Prev       string   `qstring:"before,omitempty"`
	MaxItems   int      `qstring:"maxItems,omitempty"`
	Object     *Filters `qstring:"object,omitempty"`
	Tag        *Filters `qstring:"tag,omitempty"`
	Actor      *Filters `qstring:"actor,omitempty"`
}

func EqualsString(s string) CompStr {
	return CompStr{Operator: "=", Str: s}
}

func ActivityTypesFilter(t ...vocab.ActivityVocabularyType) CompStrs {
	r := make(CompStrs, len(t))
	for i, typ := range t {
		r[i] = EqualsString(string(typ))
	}
	return r
}

func baseIRI(iri vocab.IRI) vocab.IRI {
	u, _ := iri.URL()
	u.Path = ""
	return vocab.IRI(u.String())
}

func NodeInfoResolverNew(r processing.Store, app vocab.Actor) NodeInfoResolver {
	n := NodeInfoResolver{}
	if r == nil {
		return n
	}

	//base := baseIRI(app.GetLink())
	loadFn := func(f *Filters, fn vocab.WithOrderedCollectionFn) error {
		//ff := []*Filters{{Type: CreateActivitiesFilter, Object: f}}
		//return LoadFromSearches(context.TODO(), r, RemoteLoads{base: {{loadFn: inbox, filters: ff}}}, func(ctx context.Context, c vocab.CollectionInterface, f *Filters) error {
		//	return vocab.OnOrderedCollection(c, fn)
		//})
		return nil
	}

	loadFn(actorsFilter, func(col *vocab.OrderedCollection) error {
		n.users = int(col.TotalItems)
		return nil
	})
	loadFn(postsFilter, func(col *vocab.OrderedCollection) error {
		n.posts = int(col.TotalItems)
		return nil
	})
	loadFn(allFilter, func(col *vocab.OrderedCollection) error {
		n.comments = int(col.TotalItems) - n.posts
		return nil
	})
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
	softwareName = "brutalinks"
	sourceURL    = "https://git.sr.ht/~mariusor/brutalinks"
)

func NodeInfoConfig(app vocab.Actor, ni WebInfo) nodeinfo.Config {
	return nodeinfo.Config{
		BaseURL: app.URL.GetLink().String(),
		InfoURL: "/nodeinfo",

		Metadata: nodeinfo.Metadata{
			NodeName:        string(regexp.MustCompile(`<[\/\w]+>`).ReplaceAll([]byte(ni.Title), []byte{})),
			NodeDescription: ni.Summary,
			Private:         false,
			Software: nodeinfo.SoftwareMeta{
				GitHub:   sourceURL,
				HomePage: app.URL.GetLink().String(),
				Follow:   app.AttributedTo.GetLink().String(),
			},
		},
		Protocols: []nodeinfo.NodeProtocol{
			nodeinfo.ProtocolActivityPub,
		},
		Services: nodeinfo.Services{
			Inbound:  []nodeinfo.NodeService{},
			Outbound: []nodeinfo.NodeService{nodeinfo.ServiceAtom, nodeinfo.ServiceRSS},
		},
		Software: nodeinfo.SoftwareInfo{
			Name: path.Base(softwareName),
			// TODO(marius)
			Version: "",
		},
	}
}

const selfName = "self"

func splitResourceString(res string) (string, string) {
	split := ":"
	if strings.Contains(res, "://") {
		split = "://"
	}
	ar := strings.Split(res, split)
	if len(ar) != 2 {
		return "", ""
	}
	typ := ar[0]
	handle := ar[1]
	if handle[0] == '@' && len(handle) > 1 {
		handle = handle[1:]
	}
	return typ, handle
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

func (h handler) NodeInfo() WebInfo {
	// Name formats the name of the current Application
	inf := WebInfo{
		Title:       "",
		Summary:     "Link aggregator inspired by reddit and hacker news using ActivityPub federation.",
		Description: "",
		Email:       "",
		URI:         "",
		Version:     "",
	}

	return inf
}
