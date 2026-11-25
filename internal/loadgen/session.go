package loadgen

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"

	"github.com/hmzckd/eventlens/internal/event"
)

type Product struct {
	ID       string
	Category string
	Price    uint64
}

var categories = []string{"shoes", "electronics", "books", "clothing", "home", "toys", "sports", "beauty"}

type weighted struct {
	value  string
	weight int
}

var countries = []weighted{{"TR", 70}, {"DE", 10}, {"NL", 5}, {"GB", 5}, {"US", 5}, {"AZ", 5}}
var devices = []weighted{{"mobile", 60}, {"desktop", 35}, {"tablet", 5}}

const (
	addToCartRate = 0.25
	checkoutRate  = 0.50
	purchaseRate  = 0.70
	loggedInRate  = 0.30
)

type Generator struct {
	rng      *rand.Rand
	products []Product
	zipf     *rand.Zipf
}

func NewGenerator(seed int64, productCount int) *Generator {
	rng := rand.New(rand.NewSource(seed))
	products := make([]Product, productCount)
	for i := range products {
		products[i] = Product{
			ID:       fmt.Sprintf("p-%03d", i+1),
			Category: categories[i%len(categories)],
			Price:    uint64(rng.Intn(5000)+100) * 100,
		}
	}
	return &Generator{
		rng:      rng,
		products: products,
		zipf:     rand.NewZipf(rng, 1.1, 1, uint64(productCount-1)),
	}
}

func (g *Generator) Products() []Product {
	return g.products
}

func (g *Generator) Session(now time.Time) []event.Input {
	s := session{
		id:      uuid.NewString(),
		country: pick(g.rng, countries),
		device:  pick(g.rng, devices),
		at:      now.Add(-30 * time.Second),
		rng:     g.rng,
	}
	if g.rng.Float64() < loggedInRate {
		s.user = fmt.Sprintf("u-%05d", g.rng.Intn(20000))
	}

	s.add(event.PageView, Product{}, "/")

	var cart []Product
	views := 1 + g.rng.Intn(6)
	for i := 0; i < views; i++ {
		p := g.products[g.zipf.Uint64()]
		s.add(event.ProductView, p, "/products/"+p.ID)
		if g.rng.Float64() < addToCartRate {
			s.add(event.AddToCart, p, "/products/"+p.ID)
			cart = append(cart, p)
		}
	}
	if len(cart) == 0 || g.rng.Float64() >= checkoutRate {
		return s.events
	}
	buy := cart[0]
	s.add(event.CheckoutStart, buy, "/checkout")
	if g.rng.Float64() < purchaseRate {
		s.add(event.Purchase, buy, "/checkout/done")
	}
	return s.events
}

type session struct {
	id, user, country, device string
	at                        time.Time
	rng                       *rand.Rand
	events                    []event.Input
}

func (s *session) add(typ string, p Product, url string) {
	s.at = s.at.Add(time.Duration(500+s.rng.Intn(2500)) * time.Millisecond)
	in := event.Input{
		EventID:    uuid.NewString(),
		Type:       typ,
		OccurredAt: s.at.UTC().Format(time.RFC3339Nano),
		SessionID:  s.id,
		UserID:     s.user,
		ProductID:  p.ID,
		Category:   p.Category,
		Country:    s.country,
		Device:     s.device,
		URL:        url,
	}
	if typ == event.Purchase {
		in.Price = p.Price
	}
	s.events = append(s.events, in)
}

func pick(rng *rand.Rand, options []weighted) string {
	total := 0
	for _, o := range options {
		total += o.weight
	}
	n := rng.Intn(total)
	for _, o := range options {
		if n < o.weight {
			return o.value
		}
		n -= o.weight
	}
	return options[len(options)-1].value
}
