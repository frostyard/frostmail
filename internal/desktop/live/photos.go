package live

import (
	"context"
	"encoding/base64"
	"strings"
	"sync"

	"github.com/frostyard/frostmail/api"
)

// Photos caches contacts' photos by sender address for the list, for one
// connection: the contacts' own, never fetched from anywhere (P-209). The
// first Want of an address asks maild; Get then has it, or nil for a
// sender with none. An address whose lookup failed is asked for again by
// the next Want.
type Photos struct {
	ctx    context.Context // the connection's: lookups end with it
	c      *api.Client
	notify func()

	mu       sync.Mutex
	asked    map[string]bool
	byAddr   map[string][]byte
	byPerson map[int64][]byte
}

// NewPhotos returns an empty cache for c; ctx is the connection's, and
// ends its lookups.
func NewPhotos(ctx context.Context, c *api.Client, notify func()) *Photos {
	return &Photos{ctx: ctx, c: c, notify: notify, asked: map[string]bool{}, byAddr: map[string][]byte{}, byPerson: map[int64][]byte{}}
}

// Get returns the photo of an address, nil until it has one.
func (p *Photos) Get(address string) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.byAddr[strings.ToLower(address)]
}

// Want asks maild for the photos of addresses not asked for yet.
func (p *Photos) Want(addresses []string) {
	if p.ctx.Err() != nil {
		return // the connection is gone
	}
	p.mu.Lock()
	var missing []string
	for _, a := range addresses {
		a = strings.ToLower(a)
		if !p.asked[a] {
			p.asked[a] = true
			missing = append(missing, a)
		}
	}
	p.mu.Unlock()
	if len(missing) > 0 {
		go p.load(missing)
	}
}

func (p *Photos) load(addresses []string) {
	ctx, cancel := context.WithTimeout(p.ctx, callTimeout)
	defer cancel()
	found, err := p.c.People().Senders(ctx, &api.PeopleSendersParams{Addresses: addresses})
	if err != nil {
		p.forget(addresses...)
		return
	}
	for _, f := range found {
		data, ok := p.person(ctx, f.PersonID)
		if !ok {
			p.forget(f.Address)
			continue
		}
		if data == nil {
			continue
		}
		p.mu.Lock()
		p.byAddr[strings.ToLower(f.Address)] = data
		p.mu.Unlock()
		p.notify()
	}
}

// forget lets the next Want ask for addresses again.
func (p *Photos) forget(addresses ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, a := range addresses {
		delete(p.asked, strings.ToLower(a))
	}
}

// person returns a person's photo, nil for one that does not decode; false
// when the call failed.
func (p *Photos) person(ctx context.Context, id int64) ([]byte, bool) {
	p.mu.Lock()
	data, ok := p.byPerson[id]
	p.mu.Unlock()
	if ok {
		return data, true
	}
	photo, err := p.c.People().Photo(ctx, &api.PeoplePhotoParams{ID: id})
	if err != nil {
		return nil, false
	}
	data, err = base64.StdEncoding.DecodeString(photo.Data)
	if err != nil {
		return nil, true
	}
	p.mu.Lock()
	p.byPerson[id] = data
	p.mu.Unlock()
	return data, true
}
