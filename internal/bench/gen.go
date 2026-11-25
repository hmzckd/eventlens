package bench

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"

	"github.com/hmzckd/eventlens/internal/event"
)

type Row struct {
	EventID    uuid.UUID
	Type       string
	OccurredAt time.Time
	ReceivedAt time.Time
	SessionID  string
	UserID     string
	ProductID  string
	Category   string
	Price      uint64
	Country    string
	Device     string
	URL        string
}

var (
	typeWeights = []struct {
		typ    string
		weight int
	}{
		{event.PageView, 20}, {event.ProductView, 55}, {event.AddToCart, 15},
		{event.CheckoutStart, 6}, {event.Purchase, 4},
	}
	benchCategories = []string{"shoes", "electronics", "books", "clothing", "home", "toys", "sports", "beauty"}
	benchCountries  = []string{"TR", "TR", "TR", "TR", "TR", "TR", "TR", "DE", "NL", "GB"}
	benchDevices    = []string{"mobile", "mobile", "mobile", "desktop", "desktop", "tablet"}
)

const (
	productCount = 1000
	span         = 30 * 24 * time.Hour
)

type Generator struct {
	rng      *rand.Rand
	zipf     *rand.Zipf
	end      time.Time
	sessions int
	n        int
	total    int
}

func NewGenerator(seed int64, rows int, end time.Time) *Generator {
	rng := rand.New(rand.NewSource(seed))
	return &Generator{
		rng:      rng,
		zipf:     rand.NewZipf(rng, 1.1, 1, productCount-1),
		end:      end.UTC().Truncate(time.Millisecond),
		sessions: max(rows/8, 1),
		total:    rows,
	}
}

func (g *Generator) Next() (Row, bool) {
	if g.n >= g.total {
		return Row{}, false
	}
	g.n++

	total := 0
	for _, w := range typeWeights {
		total += w.weight
	}
	pick := g.rng.Intn(total)
	typ := typeWeights[len(typeWeights)-1].typ
	for _, w := range typeWeights {
		if pick < w.weight {
			typ = w.typ
			break
		}
		pick -= w.weight
	}

	occurred := g.end.Add(-time.Duration(g.rng.Int63n(int64(span))))
	var b [16]byte
	g.rng.Read(b[:])
	id, _ := uuid.FromBytes(b[:])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80

	r := Row{
		EventID:    id,
		Type:       typ,
		OccurredAt: occurred.Truncate(time.Millisecond),
		ReceivedAt: occurred.Add(time.Duration(g.rng.Intn(3000)) * time.Millisecond).Truncate(time.Millisecond),
		SessionID:  fmt.Sprintf("s-%d", g.rng.Intn(g.sessions)),
		Country:    benchCountries[g.rng.Intn(len(benchCountries))],
		Device:     benchDevices[g.rng.Intn(len(benchDevices))],
		URL:        "/",
	}
	if g.rng.Intn(10) < 3 {
		r.UserID = fmt.Sprintf("u-%d", g.rng.Intn(100_000))
	}
	if typ != event.PageView {
		p := g.zipf.Uint64()
		r.ProductID = fmt.Sprintf("p-%04d", p+1)
		r.Category = benchCategories[p%uint64(len(benchCategories))]
		r.URL = "/products/" + r.ProductID
	}
	if typ == event.Purchase {
		r.Price = uint64(g.rng.Intn(5000)+100) * 100
	}
	return r, true
}
