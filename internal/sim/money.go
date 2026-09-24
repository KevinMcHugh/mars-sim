package sim

import "fmt"

// Money is an amount of dollars. It has no physical presence and takes no
// inventory slot: every exchange is a digital transfer between accounts.
//
// It is an integer on purpose. Settlement has to be exactly reproducible for a
// seed (see docs/determinism.md), and a float sum depends on the order its
// terms were added in. Whole dollars are enough for now; if prices ever need
// fractions, Money becomes cents and the display divides.
type Money int64

func (m Money) String() string {
	if m < 0 {
		return fmt.Sprintf("-$%d", -m)
	}
	return fmt.Sprintf("$%d", m)
}

// balance reports what an owner holds. Nobody holds nothing, and a colonist
// who is not alive reads zero: a dead colonist's money is frozen (see
// freezeWallet) and no longer anyone's to spend.
func (w *World) balance(o Owner) Money {
	switch o.Kind {
	case OwnerCommunity:
		return w.treasury
	case OwnerColonist:
		if e := w.entities[o.ID]; e != nil && e.Kind == Colonist {
			return e.wallet
		}
	}
	return 0
}

// account returns a pointer to the balance an owner's money lives in, or nil
// when the owner cannot hold money: nobody, or anyone who is not a living
// colonist.
func (w *World) account(o Owner) *Money {
	switch o.Kind {
	case OwnerCommunity:
		return &w.treasury
	case OwnerColonist:
		if e := w.entities[o.ID]; e != nil && e.Kind == Colonist {
			return &e.wallet
		}
	}
	return nil
}

// transfer moves amount from one owner to another and reports whether it
// happened. It is the only way money changes hands, so it is the one place to
// log a payment, the one place a future tax hooks in, and the reason the money
// supply can be checked at all (see moneyInCirculation).
//
// For now it refuses any transfer that would take a balance below zero. That is
// a v1 limit, not a principle: debt and lending are a planned goal (see
// docs/economy.md), and this check is the line they will relax.
func (w *World) transfer(from, to Owner, amount Money) bool {
	if amount < 0 {
		return false
	}
	src, dst := w.account(from), w.account(to)
	if src == nil || dst == nil || *src < amount {
		return false
	}
	*src -= amount // src and dst may be the same account; the net is zero
	*dst += amount
	return true
}

// mint creates money in an account. Only the founding grant and a new
// arrival's purse do this: the money supply is otherwise fixed. Every minted
// dollar is counted in moneyIssued so the supply can be audited.
func (w *World) mint(to Owner, amount Money) bool {
	if amount <= 0 {
		return amount == 0
	}
	dst := w.account(to)
	if dst == nil {
		return false
	}
	*dst += amount
	w.moneyIssued += amount
	return true
}

// freezeWallet takes a dying colonist's money out of circulation. It is kept
// on the frozen record (EntityView.Wallet) and counted in moneyFrozen, but
// nobody can spend it: inheritance needs families-as-owners, which is
// deliberately not designed yet (see docs/economy.md).
func (w *World) freezeWallet(e *Entity) {
	w.moneyFrozen += e.wallet
}

// moneyInCirculation is every spendable dollar: the treasury plus every living
// colonist's wallet. With no taxes and no sinks, it plus moneyFrozen always
// equals moneyIssued; TestMoneyIsConserved holds the colony to that.
func (w *World) moneyInCirculation() Money {
	total := w.treasury
	for _, e := range w.entities {
		if e.Kind == Colonist {
			total += e.wallet
		}
	}
	return total
}
