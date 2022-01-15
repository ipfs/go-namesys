package namesys

import (
	"context"
	"strings"
	"sync"
	"time"

	proto "github.com/gogo/protobuf/proto"
	ds "github.com/ipfs/go-datastore"
	dsquery "github.com/ipfs/go-datastore/query"
	ipns "github.com/ipfs/go-ipns"
	pb "github.com/ipfs/go-ipns/pb"
	path "github.com/ipfs/go-path"
	"github.com/libp2p/go-libp2p-core/crypto"
	peer "github.com/libp2p/go-libp2p-core/peer"
	routing "github.com/libp2p/go-libp2p-core/routing"
	base32 "github.com/whyrusleeping/base32"
	"go.uber.org/multierr"
)

const ipnsPrefix = "/ipns/"

// DefaultRecordEOL specifies the time that the network will cache IPNS
// records after being publihsed. Records should be re-published before this
// interval expires.
const DefaultRecordEOL = 24 * time.Hour

// IpnsPublisher is capable of publishing and resolving names to the IPFS
// routing system.
type IpnsPublisher struct {
	routing routing.ValueStore
	ds      ds.Datastore

	// Used to ensure we assign IPNS records *sequential* sequence numbers.
	mu sync.Mutex
}

// NewIpnsPublisher constructs a publisher for the IPFS Routing name system.
func NewIpnsPublisher(route routing.ValueStore, ds ds.Datastore) *IpnsPublisher {
	if ds == nil {
		panic("nil datastore")
	}
	return &IpnsPublisher{routing: route, ds: ds}
}

// Publish implements Publisher. Accepts a keypair and a value,
// and publishes it out to the routing system
func (p *IpnsPublisher) Publish(ctx context.Context, k crypto.PrivKey, value path.Path) error {
	return p.PublishMany(ctx, []PublishItem{{Key: k, Value: value}})
}

// PublishMany is similar to Publish except it accepts many different paths
func (p *IpnsPublisher) PublishMany(ctx context.Context, entries []PublishItem) error {
	for _, v := range entries {
		log.Debugf("Publish %s", v.Value)
	}
	return p.PublishWithEOLMany(ctx, entries, time.Now().Add(DefaultRecordEOL))
}

// IpnsDsKey returns a datastore key given an IPNS identifier (peer
// ID). Defines the storage key for IPNS records in the local datastore.
func IpnsDsKey(id peer.ID) ds.Key {
	return ds.NewKey("/ipns/" + base32.RawStdEncoding.EncodeToString([]byte(id)))
}

// ListPublished returns the latest IPNS records published by this node and
// their expiration times.
//
// This method will not search the routing system for records published by other
// nodes.
func (p *IpnsPublisher) ListPublished(ctx context.Context) (map[peer.ID]*pb.IpnsEntry, error) {
	query, err := p.ds.Query(ctx, dsquery.Query{
		Prefix: ipnsPrefix,
	})
	if err != nil {
		return nil, err
	}
	defer query.Close()

	records := make(map[peer.ID]*pb.IpnsEntry)
	for {
		select {
		case result, ok := <-query.Next():
			if !ok {
				return records, nil
			}
			if result.Error != nil {
				return nil, result.Error
			}
			e := new(pb.IpnsEntry)
			if err := proto.Unmarshal(result.Value, e); err != nil {
				// Might as well return what we can.
				log.Error("found an invalid IPNS entry:", err)
				continue
			}
			if !strings.HasPrefix(result.Key, ipnsPrefix) {
				log.Errorf("datastore query for keys with prefix %s returned a key: %s", ipnsPrefix, result.Key)
				continue
			}
			k := result.Key[len(ipnsPrefix):]
			pid, err := base32.RawStdEncoding.DecodeString(k)
			if err != nil {
				log.Errorf("ipns ds key invalid: %s", result.Key)
				continue
			}
			records[peer.ID(pid)] = e
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// GetPublished returns the record this node has published corresponding to the
// given peer ID.
//
// If `checkRouting` is true and we have no existing record, this method will
// check the routing system for any existing records.
func (p *IpnsPublisher) GetPublished(ctx context.Context, id peer.ID, checkRouting bool) (*pb.IpnsEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*30)
	defer cancel()

	value, err := p.ds.Get(ctx, IpnsDsKey(id))
	switch err {
	case nil:
	case ds.ErrNotFound:
		if !checkRouting {
			return nil, nil
		}
		ipnskey := ipns.RecordKey(id)
		value, err = p.routing.GetValue(ctx, ipnskey)
		if err != nil {
			// Not found or other network issue. Can't really do
			// anything about this case.
			if err != routing.ErrNotFound {
				log.Debugf("error when determining the last published IPNS record for %s: %s", id, err)
			}

			return nil, nil
		}
	default:
		return nil, err
	}
	e := new(pb.IpnsEntry)
	if err := proto.Unmarshal(value, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (p *IpnsPublisher) updateRecord(ctx context.Context, k crypto.PrivKey, value path.Path, eol time.Time) (*pb.IpnsEntry, error) {
	id, err := peer.IDFromPrivateKey(k)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// get previous records sequence number
	rec, err := p.GetPublished(ctx, id, true)
	if err != nil {
		return nil, err
	}

	seqno := rec.GetSequence() // returns 0 if rec is nil
	if rec != nil && value != path.Path(rec.GetValue()) {
		// Don't bother incrementing the sequence number unless the
		// value changes.
		seqno++
	}

	// Set the TTL
	// TODO: Make this less hacky.
	ttl, _ := checkCtxTTL(ctx)

	// Create record
	entry, err := ipns.Create(k, []byte(value), seqno, eol, ttl)
	if err != nil {
		return nil, err
	}

	data, err := proto.Marshal(entry)
	if err != nil {
		return nil, err
	}

	// Put the new record.
	key := IpnsDsKey(id)
	if err := p.ds.Put(ctx, key, data); err != nil {
		return nil, err
	}
	if err := p.ds.Sync(ctx, key); err != nil {
		return nil, err
	}
	return entry, nil
}

// PublishWithEOL is a temporary stand in for the ipns records implementation
// see here for more details: https://github.com/ipfs/specs/tree/master/records
func (p *IpnsPublisher) PublishWithEOL(ctx context.Context, k crypto.PrivKey, value path.Path, eol time.Time) error {
	// Use *Many route to avoid maintaining two code bases.
	return p.PublishWithEOLMany(ctx, []PublishItem{{Key: k, Value: value}}, eol)
}

type PublishItem struct {
	Key   crypto.PrivKey
	Value path.Path
}

// PublishWithEOLMany is similar to PublishWithEOL but it accepts many key + path pairs
func (p *IpnsPublisher) PublishWithEOLMany(ctx context.Context, entries []PublishItem, eol time.Time) error {
	l := len(entries)
	outEntries := make([]PutRecordToRoutingManyEntry, l)
	good := 0
	// Since error is cold path we don't need to waste time allocating if it's empty.
	var errors []error
	var err error
	for _, v := range entries {
		k := v.Key
		outEntries[good].Record, err = p.updateRecord(ctx, k, v.Value, eol)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		outEntries[good].Key = k.GetPublic()
		good++
	}

	return multierr.Combine(append(errors, PutRecordToRoutingMany(ctx, p.routing, outEntries))...)
}

// setting the TTL on published records is an experimental feature.
// as such, i'm using the context to wire it through to avoid changing too
// much code along the way.
func checkCtxTTL(ctx context.Context) (time.Duration, bool) {
	v := ctx.Value(ttlContextKey)
	if v == nil {
		return 0, false
	}

	d, ok := v.(time.Duration)
	return d, ok
}

// PutRecordToRouting publishes the given entry using the provided ValueStore,
// keyed on the ID associated with the provided public key. The public key is
// also made available to the routing system so that entries can be verified.
func PutRecordToRouting(ctx context.Context, r routing.ValueStore, k crypto.PubKey, entry *pb.IpnsEntry) error {
	return PutRecordToRoutingMany(ctx, r, []PutRecordToRoutingManyEntry{{Key: k, Record: entry}})
}

type PutRecordToRoutingManyEntry struct {
	Key    crypto.PubKey
	Record *pb.IpnsEntry
}

// PutRecordToRoutingMany is similar to PutRecordToRouting but it accepts many key + entry pairs
func PutRecordToRoutingMany(ctx context.Context, r routing.ValueStore, entries []PutRecordToRoutingManyEntry) error {
	var errors []error
	l := len(entries)
	ipnsGood := 0
	ipnsPublishEntries := make([]PublishEntryManyItem, l)
	pubkeyGood := 0
	pubkeyPublishEntries := make([]PublishPublicKeyManyEntry, l)
	for _, v := range entries {
		k := v.Key
		record := v.Record
		err := ipns.EmbedPublicKey(k, record)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		id, err := peer.IDFromPublicKey(k)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		ipnsPublishEntries[ipnsGood] = PublishEntryManyItem{Ipnskey: ipns.RecordKey(id), Record: record}
		ipnsGood++

		// Publish the public key if a public key cannot be extracted from the ID
		// TODO: once v0.4.16 is widespread enough, we can stop doing this
		// and at that point we can even deprecate the /pk/ namespace in the dht
		//
		// NOTE: This check actually checks if the public key has been embedded
		// in the IPNS entry. This check is sufficient because we embed the
		// public key in the IPNS entry if it can't be extracted from the ID.
		if record.PubKey != nil {
			pubkeyPublishEntries[pubkeyGood] = PublishPublicKeyManyEntry{Key: PkKeyForID(id), Pubkey: k}
			pubkeyGood++
		}
	}

	keysEntry, dataEntry, errEntry := preparePublishEntryMany(ipnsPublishEntries[:ipnsGood])
	keysPubkey, dataPubkey, errPubkey := preparePublishPublicKeyMany(pubkeyPublishEntries[:pubkeyGood])

	return multierr.Combine(
		append(errors, errEntry, errPubkey,
			execPut(ctx, r, append(keysEntry, keysPubkey...), append(dataEntry, dataPubkey...)),
		)...,
	)
}

// PublishPublicKey stores the given public key in the ValueStore with the
// given key.
func PublishPublicKey(ctx context.Context, r routing.ValueStore, k string, pubk crypto.PubKey) error {
	return PublishPublicKeyMany(ctx, r, []PublishPublicKeyManyEntry{{Key: k, Pubkey: pubk}})
}

type PublishPublicKeyManyEntry struct {
	Key    string
	Pubkey crypto.PubKey
}

// PublishPublicKeyMany is similar to PublishPublicKey but it accepts many key + pubkey pairs
func PublishPublicKeyMany(ctx context.Context, r routing.ValueStore, entries []PublishPublicKeyManyEntry) error {
	keys, data, err := preparePublishPublicKeyMany(entries)
	return multierr.Append(err, execPut(ctx, r, keys, data))
}

// preparePublishPublicKeyMany prepare arguments for execPut, this is usefull
// because that allows to bundle many different prepared payloads into a single PutMany
func preparePublishPublicKeyMany(entries []PublishPublicKeyManyEntry) ([]string, [][]byte, error) {
	var err error
	var errors []error
	l := len(entries)
	good := 0
	data := make([][]byte, l)
	keys := make([]string, l)
	for _, v := range entries {
		data[good], err = crypto.MarshalPublicKey(v.Pubkey)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		key := v.Key
		log.Debugf("Storing pubkey at: %s", key)
		keys[good] = key
		good++
	}
	data = data[:good]
	keys = keys[:good]

	return keys, data, multierr.Combine(errors...)
}

// PublishEntry stores the given IpnsEntry in the ValueStore with the given
// ipnskey.
func PublishEntry(ctx context.Context, r routing.ValueStore, ipnskey string, rec *pb.IpnsEntry) error {
	return PublishEntryMany(ctx, r, []PublishEntryManyItem{{Ipnskey: ipnskey, Record: rec}})
}

type putManyRouter interface {
	PutMany(ctx context.Context, keys []string, values [][]byte) error
}

type PublishEntryManyItem struct {
	Ipnskey string
	Record  *pb.IpnsEntry
}

// PublishEntryMany is similar to PublishEntry but accepts many key + entry pairs
func PublishEntryMany(ctx context.Context, r routing.ValueStore, entries []PublishEntryManyItem) error {
	keys, data, err := preparePublishEntryMany(entries)
	return multierr.Append(err, execPut(ctx, r, keys, data))
}

// preparePublishEntryMany prepare arguments for execPut, this is usefull
// because that allows to bundle many different prepared payloads into a single PutMany
func preparePublishEntryMany(entries []PublishEntryManyItem) ([]string, [][]byte, error) {
	var err error
	var errors []error
	l := len(entries)
	good := 0
	data := make([][]byte, l)
	keys := make([]string, l)
	for _, v := range entries {
		data[good], err = proto.Marshal(v.Record)
		if err != nil {
			errors = append(errors, err)
			continue
		}
		key := v.Ipnskey
		log.Debugf("Storing ipns entry at: %s", key)
		keys[good] = key
		good++
	}
	data = data[:good]
	keys = keys[:good]

	return keys, data, multierr.Combine(errors...)
}

func execPut(ctx context.Context, r routing.ValueStore, keys []string, data [][]byte) error {
	if len(keys) != len(data) {
		panic("unmatched sisterlists")
	}

	rMany, ok := r.(putManyRouter)
	if ok {
		return rMany.PutMany(ctx, keys, data)

	} else {
		goodCount := len(keys)
		switch goodCount {
		case 0:
			return nil

		case 1:
			return r.PutValue(ctx, keys[0], data[0])

		default:
			// Fallback to a parallel Put if the router doesn't support many
			var errors []error
			var wg sync.WaitGroup
			var errLock sync.Mutex
			wg.Add(goodCount)
			for goodCount != 0 {
				goodCount--
				go func(i int) {
					defer wg.Done()
					err := r.PutValue(ctx, keys[i], data[i])
					if err != nil {
						errLock.Lock()
						errors = append(errors, err)
						errLock.Unlock()
					}
				}(goodCount)
			}
			wg.Wait()
			return multierr.Combine(errors...)
		}
	}
}

// PkKeyForID returns the public key routing key for the given peer ID.
func PkKeyForID(id peer.ID) string {
	return "/pk/" + string(id)
}

// contextKey is a private comparable type used to hold value keys in contexts
type contextKey string

var ttlContextKey contextKey = "ipns-publish-ttl"

// ContextWithTTL returns a copy of the parent context with an added value representing the TTL
func ContextWithTTL(ctx context.Context, ttl time.Duration) context.Context {
	return context.WithValue(context.Background(), ttlContextKey, ttl)
}
