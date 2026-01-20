// Package securelink provides an example LinkBuilder and LinkStore that wrap
// securelink-compatible managers without adding core dependencies.
//
// Example usage (in your app):
//
//	import (
//		linksecure "github.com/goliatone/go-notifications/adapters/securelink"
//		urlsecure "github.com/goliatone/go-urlkit/securelink"
//	)
//
//	cfg := urlsecure.Config{
//		// SigningKey, BaseURL, Routes, QueryKey...
//	}
//	manager, _ := linksecure.NewManager(cfg)
//	builder := linksecure.NewBuilder(manager,
//		linksecure.WithActionRoute("reset"),
//		linksecure.WithManifestRoute("export"),
//	)
//	store := linksecure.NewMemoryStore()
//
//	module, _ := notifier.NewModule(notifier.ModuleOptions{
//		LinkBuilder: builder,
//		LinkStore:   store,
//	})
package securelink
