# Webfinger handlers on top of Go-ActivityPub storage

This project can be used as a standalone application or as a package from an external project.

Usage:

```go
	// .well-known
    cfg := NodeInfoConfig()
    ni := nodeinfo.NewService(cfg, NodeInfoResolverNew(a.front.storage))
	h := webfinger.handler{}

    // Web-Finger
    r.Route("/.well-known", func(r chi.Router) {
        r.Get("/webfinger", h.HandleWebFinger)
        r.Get("/host-meta", h.HandleHostMeta)
        r.Get("/nodeinfo", ni.NodeInfoDiscover)
    })
    r.Get("/nodeinfo", ni.NodeInfo)
 ```
