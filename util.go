package main

import (
	"sort"
	"strconv"

	"github.com/magicae/telegram-bot"
	"github.com/magicae/texas-holdem-bot/config"
)

const (
	HighCard = iota
	OnePair
	TwoPair
	ThreeOfAKind
	Straight
	Flush
	FullHouse
	FourOfAKind
	StraightFlush
	RoyalFlush
	FiveOfAKind
)

const (
	Jack  = 11
	Queen = 12
	King  = 13
	Ace   = 14
)

var PokerHands = [11]string{"HIGH CARD", "ONE PAIR", "TWO PAIRS",
	"THREE OF A KIND", "STRAIGHT", "FLUSH", "FULL HOUSE", "FOUR OF A KIND",
	"STRAIGHT FLUSH", "ROYAL FLUSH", "FIVE OF A KIND"}

func getUserDisplayName(user *bot.User) string {
	if user.LastName == "" {
		return user.FirstName
	} else {
		return user.FirstName + " " + user.LastName
	}
}

func getPokerSticker(card *PokerCard) string {
	return config.PokerFileIDs[card.Suit][card.Rank]
}

func getPokerText(card *PokerCard) string {
	return config.PokerSuitTexts[card.Suit] + config.PokerRankTexts[card.Rank]
}

func min(a int64, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

type CardSet []*PokerCard

func (c CardSet) Len() int {
	return len(c)
}

func (c CardSet) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c CardSet) Less(i, j int) bool {
	if c[i].Rank == c[j].Rank {
		return c[i].Suit < c[j].Suit
	}
	return c[i].Rank < c[j].Rank
}

func (c CardSet) GetRank() int {
	if !sort.IsSorted(c) {
		panic("Card set is not sorted.")
	}
	// Five of a kind is not possible
	straight := false
	flush := false
	// Straight
	if (c[0].Rank+1 == c[1].Rank && c[1].Rank+1 == c[2].Rank && c[2].Rank+1 == c[3].Rank && c[3].Rank+1 == c[4].Rank) ||
		(c[0].Rank == 2 && c[1].Rank == 3 && c[2].Rank == 4 && c[3].Rank == 5 && c[4].Rank == Ace) {
		straight = true
	}
	// Flush
	if c[0].Suit == c[1].Suit && c[0].Suit == c[2].Suit &&
		c[0].Suit == c[3].Suit && c[0].Suit == c[4].Suit {
		flush = true
	}
	if straight && flush {
		if c[0].Rank == 10 && c[4].Rank == Ace {
			// RoyalFlush
			return RoyalFlush
		} else {
			// StraightFlush
			return StraightFlush
		}
	}
	// FourOfAKind
	if (c[0].Rank == c[1].Rank && c[0].Rank == c[2].Rank && c[0].Rank == c[3].Rank) ||
		(c[1].Rank == c[2].Rank && c[1].Rank == c[3].Rank && c[1].Rank == c[4].Rank) {
		return FourOfAKind
	}
	// FullHouse
	if (c[0].Rank == c[1].Rank && c[0].Rank == c[2].Rank && c[3].Rank == c[4].Rank) ||
		(c[0].Rank == c[1].Rank && c[2].Rank == c[3].Rank && c[2].Rank == c[4].Rank) {
		return FullHouse
	}
	// Flush
	if flush {
		return Flush
	}
	// Straight
	if straight {
		return Straight
	}
	// ThreeOfAKind
	if (c[0].Rank == c[1].Rank && c[0].Rank == c[2].Rank) ||
		(c[1].Rank == c[2].Rank && c[1].Rank == c[3].Rank) ||
		(c[2].Rank == c[3].Rank && c[2].Rank == c[4].Rank) {
		return ThreeOfAKind
	}
	// TwoPair
	if (c[0].Rank == c[1].Rank && (c[2].Rank == c[3].Rank || c[3].Rank == c[4].Rank)) ||
		(c[1].Rank == c[2].Rank && c[3].Rank == c[4].Rank) {
		return TwoPair
	}
	// OnePair
	if c[0].Rank == c[1].Rank || c[1].Rank == c[2].Rank ||
		c[2].Rank == c[3].Rank || c[3].Rank == c[4].Rank {
		return OnePair
	}
	return HighCard
}

func LessThanHighCard(a, b CardSet) bool {
	if len(a) != len(b) {
		panic("Lengths of two card sets are not same.")
	}
	for i := len(a) - 1; i >= 0; i-- {
		if a[i].Rank != b[i].Rank {
			return a[i].Rank < b[i].Rank
		}
	}
	return false
}

func LessThanOnePair(a, b CardSet) bool {
	paira := -1
	pairb := -1
	resta := make(CardSet, 0)
	restb := make(CardSet, 0)
	for i := 0; i < 5; i++ {
		if (i < 4 && a[i].Rank == a[i+1].Rank) ||
			(i > 0 && a[i].Rank == a[i-1].Rank) {
			paira = a[i].Rank
		} else {
			resta = append(resta, a[i])
		}
	}
	for i := 0; i < 5; i++ {
		if (i < 4 && b[i].Rank == b[i+1].Rank) ||
			(i > 0 && b[i].Rank == b[i-1].Rank) {
			pairb = b[i].Rank
		} else {
			restb = append(restb, b[i])
		}
	}
	if paira != pairb {
		return paira < pairb
	}
	return LessThanHighCard(resta, restb)
}

func getPairsAndRest(a CardSet) (int, int, int) {
	high, low, rest := -1, -1, -1
	if a[3].Rank == a[4].Rank {
		high = a[3].Rank
		if a[1].Rank == a[2].Rank {
			low = a[1].Rank
			rest = a[0].Rank
		} else if a[0].Rank == a[1].Rank {
			low = a[0].Rank
			rest = a[2].Rank
		} else {
			panic("a is not two pair.")
		}
	} else if a[2].Rank == a[3].Rank {
		high = a[2].Rank
		low = a[0].Rank
		rest = a[4].Rank
	} else {
		panic("a is not two pair.")
	}
	return high, low, rest
}

func LessThanTwoPair(a, b CardSet) bool {
	ha, la, ra := getPairsAndRest(a)
	hb, lb, rb := getPairsAndRest(b)
	if ha != hb {
		return ha < hb
	}
	if la != lb {
		return la < lb
	}
	return ra < rb
}

func getThreeAndHighLow(a CardSet) (int, int, int) {
	three, high, low := -1, -1, -1
	if a[0].Rank == a[1].Rank && a[0].Rank == a[2].Rank {
		three = a[0].Rank
		low = a[3].Rank
		high = a[4].Rank
	} else if a[1].Rank == a[2].Rank && a[1].Rank == a[3].Rank {
		three = a[1].Rank
		low = a[0].Rank
		high = a[4].Rank
	} else if a[2].Rank == a[3].Rank && a[2].Rank == a[4].Rank {
		three = a[2].Rank
		low = a[0].Rank
		high = a[1].Rank
	} else {
		panic("a is not three of a kind.")
	}
	return three, high, low
}

func LessThanThreeOfAKind(a, b CardSet) bool {
	ta, ha, la := getThreeAndHighLow(a)
	tb, hb, lb := getThreeAndHighLow(b)
	if ta != tb {
		return ta < tb
	}
	if ha != hb {
		return ha < hb
	}
	return la < lb
}

func getThreeAndPair(a CardSet) (int, int) {
	three, pair := a[2].Rank, a[0].Rank
	if pair == three {
		pair = a[4].Rank
	}
	return three, pair
}

func LessThanFullHouse(a, b CardSet) bool {
	ta, pa := getThreeAndPair(a)
	tb, pb := getThreeAndPair(b)
	if ta != tb {
		return ta < tb
	}
	return pa < pb
}

func getFourAndRest(a CardSet) (int, int) {
	four, rest := a[2].Rank, a[0].Rank
	if rest == four {
		rest = a[4].Rank
	}
	return four, rest
}

func LessThanFourOfAKind(a, b CardSet) bool {
	fa, ra := getFourAndRest(a)
	fb, rb := getFourAndRest(b)
	if fa != fb {
		return fa < fb
	}
	return ra < rb
}

func LessThanCardSet(a, b CardSet) bool {
	ra, rb := a.GetRank(), b.GetRank()
	if ra != rb {
		return ra < rb
	}
	switch ra {
	case HighCard:
		return LessThanHighCard(a, b)
	case OnePair:
		return LessThanOnePair(a, b)
	case TwoPair:
		return LessThanTwoPair(a, b)
	case ThreeOfAKind:
		return LessThanThreeOfAKind(a, b)
	case Straight, StraightFlush:
		taila := a[4].Rank
		tailb := b[4].Rank
		// 2, 3, 4, 5, A
		if a[0].Rank == 2 && a[4].Rank == Ace {
			taila = a[3].Rank
		}
		// 2, 3, 4, 5, A
		if b[0].Rank == 2 && b[4].Rank == Ace {
			tailb = b[3].Rank
		}
		return taila < tailb
	case Flush:
		// Flush can be compared by high card.
		return LessThanHighCard(a, b)
	case FullHouse:
		return LessThanFullHouse(a, b)
	case FourOfAKind:
		return LessThanFourOfAKind(a, b)
	case RoyalFlush:
		return false
	default:
		panic("Unknown rank " + strconv.Itoa(ra))
	}
}

func getTopCards(communityCards [5]*PokerCard, playerCards [2]*PokerCard) CardSet {
	// Concat two sets of cards
	cards := make(CardSet, 0)
	for i := 0; i < 5; i++ {
		if communityCards[i] != nil {
			cards = append(cards, communityCards[i])
		}
	}
	if len(cards) < 3 {
		//  Less than 3 community cards
		panic("Less than 3 community cards")
	}
	for i := 0; i < 2; i++ {
		if playerCards[i] != nil {
			cards = append(cards, playerCards[i])
		} else {
			panic("Player has not enough cards")
		}
	}
	n := len(cards)

	sort.Sort(cards)
	topCards := make(CardSet, 0)
	newCards := make(CardSet, 0)

	for i := 0; i < n-4; i++ {
		for j := i + 1; j < n-3; j++ {
			for k := j + 1; k < n-2; k++ {
				for l := k + 1; l < n-1; l++ {
					for m := l + 1; m < n; m++ {
						newCards = CardSet{
							cards[i], cards[j], cards[k], cards[l], cards[m],
						}
						if len(topCards) == 0 || LessThanCardSet(topCards, newCards) {
							topCards = newCards
						}
					}
				}
			}
		}
	}

	return topCards
}
