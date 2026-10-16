package sim

import (
	"strings"
	"testing"
)

// richClient makes e afraid, armed with a rifle (so the bodyguard is the rung
// it wants next), and as rich as rich.
func richClient(w *World, e *Entity, wallet Money) {
	e.Inventory.Add(AssaultRifle, 1)
	w.syncDriveActivity(e)
	w.moneyIssued += wallet - e.wallet
	e.wallet = wallet
	uneasy(w, e, w.cfg.Drives[DriveSafety].CriticalAt+50)
}

// A rich colonist with a rifle that still feels unsafe posts an escort
// contract, escrowed from its wallet; a poor one can't afford one, and says so.
func TestOnlyTheRichHireBodyguards(t *testing.T) {
	w, _, cs := marketWorld(t)
	rich, poor := cs[0], cs[1]
	richClient(w, rich, 1000)
	richClient(w, poor, 100)

	w.postWants(rich)
	w.postWants(poor)
	o := rich.wants[DriveSafety].work
	if o == nil || o.Kind != WorkEscort || o.Units != w.cfg.EscortHours || o.Pay < Money(w.cfg.EscortWage) {
		t.Fatalf("rich colonist's contract %+v", o)
	}
	if rich.wallet != 1000-o.Pay*Money(o.Units) {
		t.Fatalf("wallet %v: the shift was not escrowed", rich.wallet)
	}
	if poor.wants[DriveSafety].work != nil {
		t.Fatal("a colonist with $100 hired a bodyguard")
	}
	if line := w.wantLine(poor); !strings.Contains(line, "can't afford") {
		t.Fatalf("poor colonist's want line %q", line)
	}
	assertMoneyConserved(t, w)
}

// A calm, armed colonist takes the contract, keeps to its client, makes the
// client feel guarded, and is paid an hour's rate from the escrow for each
// hour on station, learning to guard as it goes. An uneasy one does not take it.
func TestABodyguardWalksWithItsClient(t *testing.T) {
	w, _, cs := marketWorld(t)
	client, guard, scared := cs[0], cs[1], cs[2]
	richClient(w, client, 1000)
	w.postWants(client)
	o := client.wants[DriveSafety].work
	if o == nil {
		t.Fatal("no contract")
	}
	scared.Inventory.Add(Pistol, 1)
	uneasy(w, scared, w.cfg.Drives[DriveSafety].SeekAt+10)
	if w.tryAssignEscort(scared) {
		t.Fatal("an uneasy colonist took a bodyguard's contract")
	}
	guard.Inventory.Add(Pistol, 1)
	uneasy(w, guard, 0)
	if !w.tryAssignEscort(guard) || guard.Job != JobEscort || o.escort != guard.ID {
		t.Fatalf("the calm guard did not take it: job %v", guard.Job)
	}
	if m := client.Memories[len(client.Memories)-1]; m.Rule != "hired-bodyguard" || !strings.HasPrefix(m.Text, "Hired ") {
		t.Fatalf("client remembers %+v", m)
	}
	if m := guard.Memories[len(guard.Memories)-1]; m.Rule != "guarded-client" {
		t.Fatalf("guard remembers %+v", m)
	}

	before := guard.wallet
	hour := w.cfg.TicksPerHour()
	for i := 0; i < 3*hour && o.served < 1; i++ {
		w.tick++
		w.runJob(guard)
		w.syncDriveActivity(client)
	}
	if !w.withEscort(guard, client) {
		t.Fatalf("guard at %v, client at %v: not beside it", guard.Pos, client.Pos)
	}
	if client.place != PlaceGuarded {
		t.Fatalf("client's place %v with its bodyguard beside it", client.place)
	}
	if o.served != 1 || guard.wallet != before+o.Pay || o.Units != w.cfg.EscortHours-1 {
		t.Fatalf("served %d, paid %v, units left %d", o.served, guard.wallet-before, o.Units)
	}
	if guard.practice[SkillGuarding] == 0 {
		t.Fatal("no guarding practice for time on station")
	}
	assertMoneyConserved(t, w)
}

// Calm to serve: a bodyguard who comes to feel unsafe gives the contract up,
// which stays open for someone else; so does a defense guard its post.
func TestAnUneasyGuardGivesThePostUp(t *testing.T) {
	w, _, cs := marketWorld(t)
	client, guard := cs[0], cs[1]
	richClient(w, client, 1000)
	w.postWants(client)
	o := client.wants[DriveSafety].work
	guard.Inventory.Add(Pistol, 1)
	uneasy(w, guard, 0)
	w.tryAssignEscort(guard)
	uneasy(w, guard, w.cfg.Drives[DriveSafety].SeekAt)
	w.runJob(guard)
	if guard.Job == JobEscort || o.escort != 0 || w.workOrders[o.ID] != o {
		t.Fatalf("job %v, escort %d: the uneasy bodyguard kept the contract, or it closed", guard.Job, o.escort)
	}

	d := defenseWorld(t)
	g := d.spawn(Colonist, Point{6, 6, LandingLevel})
	g.Inventory.Add(Pistol, 1)
	uneasy(d, g, d.cfg.Drives[DriveSafety].SeekAt)
	d.orderDefense(OrderDefense{X0: 20, Y0: 8, X1: 30, Y1: 16, Detail: 1})
	if d.tryAssignGuard(g) {
		t.Fatal("an uneasy colonist took a defense post")
	}
}

// A contract nobody takes lapses after want-ttl: the escrow comes back, and the
// client counts a lapse and decides again.
func TestAnUntakenContractLapses(t *testing.T) {
	w, _, cs := marketWorld(t)
	client := cs[0]
	richClient(w, client, 1000)
	w.postWants(client)
	o := client.wants[DriveSafety].work
	w.tick = o.lapseAt
	w.postWants(client)
	if w.workOrders[o.ID] != nil {
		t.Fatal("the contract did not lapse")
	}
	if ws := client.wants[DriveSafety]; ws.lapses != 1 || ws.work == nil || ws.work == o {
		t.Fatalf("after a lapse: %+v, want one lapse and a fresh contract", ws)
	}
	assertMoneyConserved(t, w)
}

// Each guarding rank cuts the safety a colonist feels, rate and sights alike.
func TestGuardingSteadiesNerve(t *testing.T) {
	w, _, cs := marketWorld(t)
	vet, rookie := cs[0], cs[1]
	vet.practice[SkillGuarding] = skillSpecs[SkillGuarding].threshold(2)
	for _, e := range []*Entity{vet, rookie} {
		w.syncDriveActivity(e)
		w.setDrive(e, DriveSafety, 0)
	}
	if v, r := vet.drives[DriveSafety].rate, rookie.drives[DriveSafety].rate; v != r*(100-2*w.cfg.GuardingNervePercent)/100 {
		t.Fatalf("rank-2 veteran's rate %d, rookie's %d", v, r)
	}
	for _, e := range []*Entity{vet, rookie} {
		e.drives[DriveSafety].rate = 0
		w.rememberPercept(e, killingSeen(w, e, 98))
		e.drives[DriveSafety].rate = 0
	}
	if v, r := safetyLevel(w, vet), safetyLevel(w, rookie); v != r*(100-2*w.cfg.GuardingNervePercent)/100 {
		t.Fatalf("veteran took a killing at %d, rookie at %d", v, r)
	}
}
