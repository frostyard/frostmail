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
// sender with none.
type Photos struct {
	c      *api.Client
	notify func()

	mu       sync.Mutex
	asked    map[string]bool
	byAddr   map[string][]byte
	byPerson map[int64][]byte
}

// NewPhotos returns an empty cache for c.
func NewPhotos(c *api.Client, notify func()) *Photos {
	return &Photos{c: c, notify: notify, asked: map[string]bool{}, byAddr: map[string][]byte{}, byPerson: map[int64][]byte{}}
}

// Get returns the photo of an address, nil until it has one.
func (p *Photos) Get(address string) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.byAddr[strings.ToLower(address)]
}

// Want asks maild for the photos of addresses not asked for yet.
func (p *Photos) Want(addresses []string) {
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
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	found, err := p.c.People().Senders(ctx, &api.PeopleSendersParams{Addresses: addresses})
	if err != nil {
		return
	}
	for _, f := range found {
		data := p.person(ctx, f.PersonID)
		if data == nil {
			continue
		}
		p.mu.Lock()
		p.byAddr[strings.ToLower(f.Address)] = data
		p.mu.Unlock()
		p.notify()
	}
}

func (p *Photos) person(ctx context.Context, id int64) []byte {
	p.mu.Lock()
	data, ok := p.byPerson[id]
	p.mu.Unlock()
	if ok {
		return data
	}
	photo, err := p.c.People().Photo(ctx, &api.PeoplePhotoParams{ID: id})
	if err != nil {
		return nil
	}
	data, err = base64.StdEncoding.DecodeString(photo.Data)
	if err != nil {
		return nil
	}
	p.mu.Lock()
	p.byPerson[id] = data
	p.mu.Unlock()
	return data
}
