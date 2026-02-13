# Webfinger handlers on top of Go-ActivityPub storage

This project is a standalone server for providing WellKnown support for GoActivityPub servers that don't expose that functionality themselves.

It was created as a "sidecar" service that would work alongside [FedBOX](https://github.com/go-ap/fedbox).

Please see [the official documentation](https://mariusor.srht.site/apps/fedbox/well-known/) for how to configure and use it.

As a library you can use it like this:

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

