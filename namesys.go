package namesys

import (
	lib "github.com/ipfs/go-ipfs/lib/namesys"
)

var ErrResolveFailed = lib.ErrResolveFailed
var ErrResolveRecursion = lib.ErrResolveRecursion
var ErrPublishFailed = lib.ErrPublishFailed

type NameSystem = lib.NameSystem
type Result = lib.Result
type Resolver = lib.Resolver
type Publisher = lib.Publisher
