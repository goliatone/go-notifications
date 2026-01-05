// Package securelink provides an example LinkBuilder and LinkStore that wrap
// securelink-compatible managers without adding core dependencies.
//
// Example usage (in your app):
//
//	import (
//		linksecure "github.com/goliatone/go-notifications/adapters/securelink"
//		"github.com/goliatone/go-urlkit/securelink"
//	)
//
//	manager := securelink.NewManager(cfg)
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
