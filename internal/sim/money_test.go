package sim

import (
	"math/rand"
	"testing"
)

// The founding grant seeds the treasury and every arrival mints a purse, so a
// fresh colony's issued money is exactly those two terms.
func TestFoundingGrantAndPursesAreMinted(t *testing.T) {
	cfg := testConfig()
	w := newTestWorld(t, cfg)

	if w.treasury != Money(cfg.FoundingGrant) {
		t.Fatalf("treasury: got %v want %v", w.treasury, Money(cfg.FoundingGrant))
	}
	for _, id := range w.entityIDsSorted() {
		if e := w.entities[id]; e.Kind == Colonist && e.wallet != Money(cfg.CrashPodPurse) {
			t.Fatalf("%s arrived with %v, want %v", e.displayName(), e.wallet, Money(cfg.CrashPodPurse))
		}
	}
	want := Money(cfg.FoundingGrant) + Money(cfg.CrashPodPurse)*Money(cfg.StartColonists)
	if w.moneyIssued != want {
		t.Fatalf("issued: got %v want %v", w.moneyIssued, want)
	}

	// A colonist spawned mid-game (the spawn command) gets a purse too.
	late := w.spawn(Colonist, openFloor(t, w))
	if late.wallet != Money(cfg.CrashPodPurse) || w.moneyIssued != want+Money(cfg.CrashPodPurse) {
		t.Fatalf("late arrival: wallet %v issued %v", late.wallet, w.moneyIssued)
	}
}

func TestTransferRules(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists = 0
	cfg.FoundingGrant, cfg.CrashPodPurse = 1000, 50
	w := newTestWorld(t, cfg)
	a := w.spawn(Colonist, openFloor(t, w))
	b := w.spawn(Colonist, openFloor(t, w))
	A, B := ColonistOwner(a.ID), ColonistOwner(b.ID)

	if !w.transfer(Community, A, 200) || a.wallet != 250 || w.treasury != 800 {
		t.Fatalf("treasury -> a: a=%v treasury=%v", a.wallet, w.treasury)
	}
	if !w.transfer(A, B, 250) || a.wallet != 0 || b.wallet != 300 {
		t.Fatalf("a -> b, whole balance: a=%v b=%v", a.wallet, b.wallet)
	}
	// No balance may go negative: this is the v1 no-debt rule.
	if w.transfer(A, B, 1) || a.wallet != 0 || b.wallet != 300 {
		t.Fatalf("overdraft allowed: a=%v b=%v", a.wallet, b.wallet)
	}
	if w.transfer(B, A, -5) {
		t.Fatal("negative transfer allowed")
	}
	// Nobody can hold money, in either direction.
	if w.transfer(B, Nobody, 10) || w.transfer(Nobody, B, 10) {
		t.Fatal("transfer involving nobody allowed")
	}
	// Paying yourself is a no-op that still needs the funds.
	if !w.transfer(B, B, 300) || b.wallet != 300 || w.transfer(B, B, 301) {
		t.Fatalf("self transfer: b=%v", b.wallet)
	}
	// A dead colonist's account is closed.
	w.remove(b.ID, "test")
	if w.transfer(Community, B, 10) || w.transfer(B, Community, 10) {
		t.Fatal("transfer involving a dead colonist allowed")
	}
	if w.balance(B) != 0 {
		t.Fatalf("dead colonist still reports a balance: %v", w.balance(B))
	}
}

// Death takes a wallet out of circulation but keeps it on the record.
func TestDeathFreezesWallet(t *testing.T) {
	cfg := testConfig()
	cfg.StartColonists = 0
	w := newTestWorld(t, cfg)
	a := w.spawn(Colonist, openFloor(t, w))
	w.transfer(Community, ColonistOwner(a.ID), 40)
	held := a.wallet

	w.remove(a.ID, "test")
	if w.moneyFrozen != held {
		t.Fatalf("frozen: got %v want %v", w.moneyFrozen, held)
	}
	if got := w.deceasedColonists[a.ID].Wallet; got != held {
		t.Fatalf("deceased record wallet: got %v want %v", got, held)
	}
	assertMoneyConserved(t, w)
}

// With no taxes and no sinks, every dollar ever minted is either spendable or
// frozen with the dead. This runs a live colony (arrivals, deaths to aliens)
// while shuffling money between random accounts every tick, and checks the
// books balance on every one of them. Whether the aliens kill anyone depends
// on the seed, so the test also kills a colonist itself partway through:
// that keeps freezing covered whatever worldgen does to the seed.
func TestMoneyIsConserved(t *testing.T) {
	cfg := testConfig()
	cfg.Seed = 7
	cfg.Width, cfg.Height = 80, 50
	cfg.StartColonists = 16
	w := newTestWorld(t, cfg)
	r := rand.New(rand.NewSource(1)) // the test's own stream, never the world's

	for i := 0; i < 3000; i++ {
		w.step()
		if i%500 == 0 {
			if p, ok := w.randomFloor(); ok && !w.occupied(p) {
				w.spawn(Colonist, p)
			}
		}
		if i == 1500 {
			for _, id := range w.entityIDsSorted() {
				if e := w.entities[id]; e.Kind == Colonist && e.wallet > 0 {
					w.remove(id, "test")
					break
				}
			}
		}
		owners := []Owner{Community, Nobody}
		for _, id := range w.entityIDsSorted() {
			if w.entities[id].Kind == Colonist {
				owners = append(owners, ColonistOwner(id))
			}
		}
		for k := 0; k < 4; k++ {
			from, to := owners[r.Intn(len(owners))], owners[r.Intn(len(owners))]
			w.transfer(from, to, Money(r.Intn(300)))
		}
		assertMoneyConserved(t, w)
	}
	if w.moneyFrozen == 0 {
		t.Fatal("expected some colonist to die holding money; the test no longer covers freezing")
	}
}

func assertMoneyConserved(t *testing.T, w *World) {
	t.Helper()
	if got := w.moneyInCirculation() + w.moneyFrozen + w.moneyEscrowed(); got != w.moneyIssued {
		t.Fatalf("tick %d: circulating %v + frozen %v + escrowed %v = %v, issued %v",
			w.tick, w.moneyInCirculation(), w.moneyFrozen, w.moneyEscrowed(), got, w.moneyIssued)
	}
	for _, o := range w.orders {
		if o.Side == Bid && o.escrow != Money(o.Qty)*o.Price {
			t.Fatalf("tick %d: bid %d escrows %v for %d at %v", w.tick, o.ID, o.escrow, o.Qty, o.Price)
		}
	}
	for _, e := range w.entities {
		if e.wallet < 0 {
			t.Fatalf("tick %d: %s has a negative wallet %v", w.tick, e.displayName(), e.wallet)
		}
	}
	if w.treasury < 0 {
		t.Fatalf("tick %d: negative treasury %v", w.tick, w.treasury)
	}
}

// openFloor finds an empty walkable tile for a test to spawn onto.
func openFloor(t *testing.T, w *World) Point {
	t.Helper()
	for y := 0; y < w.Height; y++ {
		for x := 0; x < w.Width; x++ {
			if p := (Point{x, y}); w.Walkable(p) && !w.occupied(p) {
				return p
			}
		}
	}
	t.Fatal("no open floor")
	return Point{}
}
