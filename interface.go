/*
Package namesys implements resolvers and publishers for the IPFS
naming system (IPNS).

The core of IPFS is an immutable, content-addressable Merkle graph.
That works well for many use cases, but doesn't allow you to answer
questions like "what is Alice's current homepage?".  The mutable name
system allows Alice to publish information like:

	The current homepage for alice.example.com is
	/ipfs/Qmcqtw8FfrVSBaRmbWwHxt3AuySBhJLcvmFYi3Lbc4xnwj

or:

	The current homepage for node
	QmatmE9msSfkKxoffpHwNLNKgwZG8eT9Bud6YoPab52vpy
	is
	/ipfs/Qmcqtw8FfrVSBaRmbWwHxt3AuySBhJLcvmFYi3Lbc4xnwj

The mutable name system also allows users to resolve those references
to find the immutable IPFS object currently referenced by a given
mutable name.

For command-line bindings to this functionality, see:

	ipfs name
	ipfs dns
	ipfs resolve
*/package namesys

import (
	"errors"

	"context"

	"github.com/ipfs/go-path"
	opts "github.com/ipfs/interface-go-ipfs-core/options/namesys"
	ci "github.com/libp2p/go-libp2p/core/crypto"
)

// ErrResolveFailed signals an error when attempting to resolve.
//
// Deprecated: use github.com/ipfs/boxo/namesys.ErrResolveFailed
var ErrResolveFailed = errors.New("could not resolve name")

// ErrResolveRecursion signals a recursion-depth limit.
//
// Deprecated: use github.com/ipfs/boxo/namesys.ErrResolveRecursion
var ErrResolveRecursion = errors.New(
	"could not resolve name (recursion limit exceeded)")

// ErrPublishFailed signals an error when attempting to publish.
//
// Deprecated: use github.com/ipfs/boxo/namesys.ErrPublishFailed
var ErrPublishFailed = errors.New("could not publish name")

// NameSystem represents a cohesive name publishing and resolving system.
//
// Publishing a name is the process of establishing a mapping, a key-value
// pair, according to naming rules and databases.
//
// Resolving a name is the process of looking up the value associated with the
// key (name).
//
// Deprecated: use github.com/ipfs/boxo/namesys.NameSystem
type NameSystem interface {
	Resolver
	Publisher
}

// Result is the return type for Resolver.ResolveAsync.
//
// Deprecated: use github.com/ipfs/boxo/namesys.Result
type Result struct {
	Path path.Path
	Err  error
}

// Resolver is an object capable of resolving names.
//
// Deprecated: use github.com/ipfs/boxo/namesys.Resolver
type Resolver interface {

	// Resolve performs a recursive lookup, returning the dereferenced
	// path.  For example, if ipfs.io has a DNS TXT record pointing to
	//   /ipns/QmatmE9msSfkKxoffpHwNLNKgwZG8eT9Bud6YoPab52vpy
	// and there is a DHT IPNS entry for
	//   QmatmE9msSfkKxoffpHwNLNKgwZG8eT9Bud6YoPab52vpy
	//   -> /ipfs/Qmcqtw8FfrVSBaRmbWwHxt3AuySBhJLcvmFYi3Lbc4xnwj
	// then
	//   Resolve(ctx, "/ipns/ipfs.io")
	// will resolve both names, returning
	//   /ipfs/Qmcqtw8FfrVSBaRmbWwHxt3AuySBhJLcvmFYi3Lbc4xnwj
	//
	// There is a default depth-limit to avoid infinite recursion.  Most
	// users will be fine with this default limit, but if you need to
	// adjust the limit you can specify it as an option.
	Resolve(ctx context.Context, name string, options ...opts.ResolveOpt) (value path.Path, err error)

	// ResolveAsync performs recursive name lookup, like Resolve, but it returns
	// entries as they are discovered in the DHT. Each returned result is guaranteed
	// to be "better" (which usually means newer) than the previous one.
	ResolveAsync(ctx context.Context, name string, options ...opts.ResolveOpt) <-chan Result
}

// Publisher is an object capable of publishing particular names.
//
// Deprecated: use github.com/ipfs/boxo/namesys.Publisher
type Publisher interface {
	// Publish establishes a name-value mapping.
	// TODO make this not PrivKey specific.
	Publish(ctx context.Context, name ci.PrivKey, value path.Path, options ...opts.PublishOption) error
}
