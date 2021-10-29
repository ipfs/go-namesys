package republisher

import (
	lib "github.com/ipfs/go-ipfs/lib/namesys/republisher"
)

var DefaultRebroadcastInterval = lib.DefaultRebroadcastInterval
var InitialRebroadcastDelay = lib.InitialRebroadcastDelay
var FailureRetryInterval = lib.FailureRetryInterval

const DefaultRecordLifetime = lib.DefaultRecordLifetime

type Republisher = lib.Republisher

var NewRepublisher = lib.NewRepublisher
