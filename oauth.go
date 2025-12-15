package webfinger

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	vocab "github.com/go-ap/activitypub"
	"github.com/go-ap/auth"
	"github.com/go-ap/errors"
	"github.com/go-ap/filters"
	"github.com/openshift/osin"
)

// OAuthAuthorizationMetadata is the metadata returned by RFC8414 well known oauth-authorization-server end-point
//
// https://datatracker.ietf.org/doc/html/rfc8414#section-3.2
type OAuthAuthorizationMetadata struct {
	Issuer                                     string                   `json:"issuer"`
	AuthorizationEndpoint                      string                   `json:"authorization_endpoint"`
	TokenEndpoint                              string                   `json:"token_endpoint"`
	TokenEndpointAuthMethodsSupported          []string                 `json:"token_endpoint_auth_methods_supported,omitempty"`
	TokenEndpointAuthSigningAlgValuesSupported []string                 `json:"token_endpoint_auth_signing_alg_values_supported,omitempty"`
	RegistrationEndpoint                       string                   `json:"registration_endpoint"`
	GrantTypesSupported                        []osin.AccessRequestType `json:"grant_types_supported,omitempty"`
	ScopesSupported                            []string                 `json:"scopes_supported,omitempty"`
	ResponseTypesSupported                     []string                 `json:"response_types_supported,omitempty"`
}

func defaultGrantTypes() []osin.AccessRequestType {
	grants := make([]osin.AccessRequestType, 0, len(auth.DefaultAccessTypes))
	for _, typ := range auth.DefaultAccessTypes {
		if typ == osin.IMPLICIT {
			typ = "implicit"
		}
		grants = append(grants, typ)
	}
	return grants
}

const WellKnownOAuthAuthorizationServerPath = "/.well-known/oauth-authorization-server"

func issuerIRIFromRequest(req *http.Request) vocab.IRI {
	maybeActorURI := "https://" + req.Host + strings.Replace(req.RequestURI, WellKnownOAuthAuthorizationServerPath, "", 1)
	return vocab.IRI(maybeActorURI)
}

func clientRegistrationIRI(self vocab.Actor) string {
	fallBack := self.ID.AddPath("oauth/client").String()
	if self.Endpoints == nil || vocab.IsNil(self.Endpoints.OauthTokenEndpoint) {
		return fallBack
	}
	tokURL, err := self.Endpoints.OauthTokenEndpoint.GetID().URL()
	if err != nil {
		return fallBack
	}
	tokURL.Path = filepath.Join(filepath.Dir(tokURL.Path), "client")
	return tokURL.String()
}

// HandleOAuthAuthorizationServer serves /.well-known/oauth-authorization-server
func (h handler) HandleOAuthAuthorizationServer(w http.ResponseWriter, r *http.Request) {
	maybeActor, err := LoadActor(h.s, filters.SameID(issuerIRIFromRequest(r)))
	if err != nil {
		handleErr(h.l)(r, errors.Annotatef(err, "unable to determine issuer")).ServeHTTP(w, r)
		return
	}
	if auth.AnonymousActor.Equals(maybeActor) {
		handleErr(h.l)(r, errors.NotFoundf("issuer not found")).ServeHTTP(w, r)
		return
	}
	// NOTE(marius): this implies that only actors can be OAuth2 issuers
	self, err := vocab.ToActor(maybeActor)
	if err != nil {
		handleErr(h.l)(r, errors.Annotatef(err, "invalid type for issuer %T", maybeActor)).ServeHTTP(w, r)
		return
	}
	meta := OAuthAuthorizationMetadata{
		Issuer:                            self.ID.String(),
		GrantTypesSupported:               defaultGrantTypes(),
		TokenEndpointAuthMethodsSupported: []string{"client_secret_basic"},
		// NOTE(marius): This URL is not handled by us, as it's related to the OAuth2 authorization flow.
		// It is exposed by the git.sr.ht/~mariusor/authorize service.
		// TODO(marius): find a way to unify the way we generate this IRI
		RegistrationEndpoint:                       clientRegistrationIRI(*self),
		TokenEndpointAuthSigningAlgValuesSupported: []string{},
		ResponseTypesSupported:                     nil,
	}
	if self.Endpoints != nil {
		if !vocab.IsNil(self.Endpoints.OauthAuthorizationEndpoint) {
			meta.AuthorizationEndpoint = self.Endpoints.OauthAuthorizationEndpoint.GetID().String()
		}
		if !vocab.IsNil(self.Endpoints.OauthTokenEndpoint) {
			meta.TokenEndpoint = self.Endpoints.OauthTokenEndpoint.GetID().String()
		}
	}
	if meta.AuthorizationEndpoint == "" {
		meta.AuthorizationEndpoint = self.ID.AddPath("oauth/authorize").String()
	}
	if meta.TokenEndpoint == "" {
		meta.TokenEndpoint = self.ID.AddPath("oauth/token").String()
	}
	data, _ := json.Marshal(meta)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	h.l.Debugf("%s %s%s %d %s", r.Method, r.Host, r.RequestURI, http.StatusOK, http.StatusText(http.StatusOK))
}
