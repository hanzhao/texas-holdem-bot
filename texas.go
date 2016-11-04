package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sort"
	"strconv"

	. "github.com/magicae/telegram-bot"
	"gopkg.in/redis.v5"
)

// Suits of poker
const (
	Diamonds = iota
	Hearts
	Clubs
	Spades
)

// Round stage
const (
	Init = iota
	CompulsoryBets
	Preflop
	Flop
	Turn
	River
	Showdown
	End
)

// User state
const (
	Out = iota
	InGame
	Fold
)

var StageNames []string = []string{"Init", "Compulsory Bets", "Preflop", "Flop",
	"Turn", "River", "Showdown", "End"}

type (
	Texas struct {
		Bot     *Bot
		ChatID  int64
		Players [10]*TexasPlayer
		Dealer  int
		MaxChip int64
		Round   *Round
	}

	TexasPlayer struct {
		UserID      int
		DisplayName string
		Username    string
		Chip        int64
	}

	// TexasDealer deals poker cards to everyone.
	TexasDealer struct {
		CardSet []*PokerCard
	}

	PokerCard struct {
		Suit int
		// Rank 2-14 denotes 2, 3, 4, 5, 6, 7, 8, 9, 10, Jack, Queen, King, Ace.
		Rank int
	}

	Round struct {
		Pot            int64
		Dealer         int
		Stage          int
		CardDealer     *TexasDealer
		CommunityCards [5]*PokerCard
		UserState      [10]int
		PlayerCards    [10][2]*PokerCard
		TopCards       [10]CardSet
		TotalBets      [10]int64
		StageBets      [10]int64
		Earn           [10]int64
		ActorIndex     int
		LastRaiser     int
	}

	PlayerHand struct {
		Hand  CardSet
		Index int
		Bets  int64
	}

	PlayerHands []*PlayerHand
)

// Create a new game for everyone.
func NewTexas(e *Bot, chatID int64, maxChip int64) *Texas {
	texas := &Texas{
		Bot:     e,
		ChatID:  chatID,
		Dealer:  0,
		MaxChip: maxChip,
	}
	return texas
}

// Create a new dealer for a new card pack.
func NewTexasDealer() *TexasDealer {
	dealer := &TexasDealer{
		CardSet: make([]*PokerCard, 52),
	}
	cards := make([]*PokerCard, 52)
	count := 0
	for suit := 0; suit < 4; suit++ {
		for rank := 2; rank <= 14; rank++ {
			cards[count] = &PokerCard{suit, rank}
			count += 1
		}
	}
	// Shuffle
	perm := rand.Perm(len(cards))
	for i, v := range perm {
		dealer.CardSet[i] = cards[v]
	}
	return dealer
}

// Deal a new card.
func (d *TexasDealer) Deal() *PokerCard {
	n := len(d.CardSet)
	r := rand.Intn(n)
	selected := d.CardSet[r]
	// Remove card from cardset.
	d.CardSet[r] = d.CardSet[len(d.CardSet)-1]
	d.CardSet = d.CardSet[:len(d.CardSet)-1]
	return selected
}

// Add a user into the game. Returns the chips bought.
func (t *Texas) AddUser(user *User) (int64, error) {
	chatKey := "texas:user:" + strconv.Itoa(user.ID) + ":chat"
	_, err := redisClient.Get(chatKey).Result()
	if err == redis.Nil {
		return 0, errors.New("You need /start in the private chat at first!")
	}
	for i := 0; i < 10; i++ {
		if t.Players[i] != nil && t.Players[i].UserID == user.ID {
			return 0, errors.New("You have been in this game.")
		}
	}
	for i := 0; i < 10; i++ {
		// Find an empty seat.
		if t.Players[i] == nil {
			// Get the user's money.
			moneyKey := "texas:user:" + strconv.Itoa(user.ID) + ":money"
			chip, err := redisClient.Get(moneyKey).Int64()
			if err != nil && err != redis.Nil {
				return 0, err
			}
			if err == redis.Nil || chip <= 0 {
				return 0, errors.New("You are too poor to join game.")
			}
			// Determine how many chips he can buy.
			buy := min(chip, t.MaxChip)
			// Decrease the user's money.
			err = redisClient.DecrBy(moneyKey, buy).Err()
			if err != nil {
				return 0, err
			}
			// Add to the game.
			t.Players[i] = &TexasPlayer{
				UserID:      user.ID,
				DisplayName: getUserDisplayName(user),
				Username:    user.Username,
				Chip:        buy,
			}
			return buy, nil
		}
	}
	return 0, errors.New("There are no seats for you.")
}

// Remove a user from the game. Returns the chips returned.
func (t *Texas) RemoveUser(user *User) (int64, error) {
	for i := 0; i < 10; i++ {
		// Find the seat the user sat.
		if t.Players[i] != nil && t.Players[i].UserID == user.ID {
			// Return money to the user.
			get := t.Players[i].Chip
			moneyKey := "texas:user:" + strconv.Itoa(user.ID) + ":money"
			err := redisClient.IncrBy(moneyKey, get).Err()
			if err != nil {
				return 0, err
			}
			t.Players[i] = nil
			return get, nil
		}
	}
	return 0, errors.New("You are currently not in this game.")
}

// Count players.
func (t *Texas) CountUser() int {
	count := 0
	for i := 0; i < 10; i++ {
		if t.Players[i] != nil {
			count++
		}
	}
	return count
}

// Count valid players
func (t *Texas) CountUserInGame() int {
	count := 0
	for i := 0; i < 10; i++ {
		if t.Players[i] != nil && t.Round.UserState[i] == InGame {
			count++
		}
	}
	return count
}

// Create a now round.
func (t *Texas) StartRound() error {
	if t.Round != nil && t.Round.Stage != End {
		return errors.New("This round is still taking.")
	}
	// TODO: add this
	if t.CountUser() < 2 {
		return errors.New("Not enough players to start a new round.")
	}
	// Find the next dealer.
	dealer := -1
	for i := 1; i <= 10; i++ {
		if t.Players[(t.Dealer+i)%10] != nil {
			dealer = (t.Dealer + i) % 10
			break
		}
	}
	// Change dealer.
	t.Dealer = dealer
	// Create new round.
	t.Round = &Round{
		Pot:        0,
		Dealer:     t.Dealer,
		Stage:      Init,
		CardDealer: NewTexasDealer(),
	}
	// Set players as valid
	for i := 0; i < 10; i++ {
		if t.Players[i] != nil {
			t.Round.UserState[i] = InGame
		} else {
			t.Round.UserState[i] = Out
		}
	}
	return nil
}

func (t *Texas) SendMaxHand(stage int) {
	// Notify max rank
	for i := 0; i < 10; i++ {
		if t.Round.UserState[i] == InGame {
			chatKey := "texas:user:" + strconv.Itoa(t.Players[i].UserID) +
				":chat"
			chatID, err := redisClient.Get(chatKey).Int64()
			if err != nil {
				log.Println("Error: ", err, "< SendMaxHand")
				continue
			}
			t.Round.TopCards[i] = getTopCards(t.Round.CommunityCards,
				t.Round.PlayerCards[i])
			_, err = t.Bot.SendMessage(&SendMessageRequest{
				ChatID: chatID,
				Text: "[" + StageNames[stage] + "] You got " +
					PokerHands[t.Round.TopCards[i].GetRank()] + "!",
			})
			if err != nil {
				log.Println("Error: ", err, "< SendMaxHand")
				continue
			}
		}
	}
}

// Move on.
func (t *Texas) MoveOn() error {
	if t.Round == nil {
		return errors.New("Round is not ready.")
	}
	switch t.Round.Stage {
	case Init:
		// Deal cards to every one.
		for i := 0; i < 10; i++ {
			if t.Round.UserState[i] == InGame {
				chatKey := "texas:user:" + strconv.Itoa(t.Players[i].UserID) +
					":chat"
				chatID, err := redisClient.Get(chatKey).Int64()
				if err != nil {
					return err
				}
				_, err = t.Bot.SendMessage(&SendMessageRequest{
					ChatID: chatID,
					Text:   "New round starts! Dealing for you. Good luck!",
				})
				if err != nil {
					return err
				}
				t.Round.Earn[i] = 0
				t.Round.CardDealer.Deal() // Dealer skips a card.
				t.Round.PlayerCards[i][0] = t.Round.CardDealer.Deal()
				go t.Bot.SendSticker(&SendStickerRequest{
					ChatID:  chatID,
					Sticker: getPokerSticker(t.Round.PlayerCards[i][0]),
				})
				t.Round.CardDealer.Deal() // Dealer skips a card.
				t.Round.PlayerCards[i][1] = t.Round.CardDealer.Deal()
				go t.Bot.SendSticker(&SendStickerRequest{
					ChatID:  chatID,
					Sticker: getPokerSticker(t.Round.PlayerCards[i][1]),
				})
			}
		}
		t.Round.Stage = CompulsoryBets
		return t.MoveOn()
	case CompulsoryBets:
		for i := 0; i < 10; i++ {
			t.Round.StageBets[i] = 0
		}
		smallBlind := t.Round.NextValidIndex(t.Round.Dealer)
		t.MakeBet(smallBlind, 50)
		bigBlind := t.Round.NextValidIndex(smallBlind)
		t.MakeBet(bigBlind, 100)
		t.Round.ActorIndex = t.Round.NextValidIndex(bigBlind)
		t.Round.LastRaiser = t.Round.NextValidIndex(bigBlind)
		t.Round.Stage = Preflop
		return t.MoveOn()
	case Preflop:
		return t.ShowStatus()
	case Flop:
		for i := 0; i < 10; i++ {
			t.Round.StageBets[i] = 0
		}
		t.Round.CardDealer.Deal() // Dealer skips a card.
		t.Round.CommunityCards[0] = t.Round.CardDealer.Deal()
		// TODO: error handler
		t.Bot.SendSticker(&SendStickerRequest{
			ChatID:  t.ChatID,
			Sticker: getPokerSticker(t.Round.CommunityCards[0]),
		})
		t.Round.CardDealer.Deal() // Dealer skips a card.
		t.Round.CommunityCards[1] = t.Round.CardDealer.Deal()
		// TODO: error handler
		t.Bot.SendSticker(&SendStickerRequest{
			ChatID:  t.ChatID,
			Sticker: getPokerSticker(t.Round.CommunityCards[1]),
		})
		t.Round.CardDealer.Deal() // Dealer skips a card.
		t.Round.CommunityCards[2] = t.Round.CardDealer.Deal()
		// TODO: error handler
		t.Bot.SendSticker(&SendStickerRequest{
			ChatID:  t.ChatID,
			Sticker: getPokerSticker(t.Round.CommunityCards[2]),
		})
		t.Round.LastRaiser = t.Round.NextValidIndex(t.Round.Dealer)
		t.Round.ActorIndex = t.Round.Dealer
		t.SendMaxHand(t.Round.Stage)
		return t.NextPlayer(1)
	case Turn:
		for i := 0; i < 10; i++ {
			t.Round.StageBets[i] = 0
		}
		t.Round.CardDealer.Deal() // Dealer skips a card.
		t.Round.CommunityCards[3] = t.Round.CardDealer.Deal()
		// TODO: error handler
		t.Bot.SendSticker(&SendStickerRequest{
			ChatID:  t.ChatID,
			Sticker: getPokerSticker(t.Round.CommunityCards[3]),
		})
		t.Round.LastRaiser = t.Round.NextValidIndex(t.Round.Dealer)
		t.Round.ActorIndex = t.Round.Dealer
		t.SendMaxHand(t.Round.Stage)
		return t.NextPlayer(1)
	case River:
		for i := 0; i < 10; i++ {
			t.Round.StageBets[i] = 0
		}
		t.Round.CardDealer.Deal() // Dealer skips a card.
		t.Round.CommunityCards[4] = t.Round.CardDealer.Deal()
		// TODO: error handler
		t.Bot.SendSticker(&SendStickerRequest{
			ChatID:  t.ChatID,
			Sticker: getPokerSticker(t.Round.CommunityCards[4]),
		})
		t.Round.LastRaiser = t.Round.NextValidIndex(t.Round.Dealer)
		t.Round.ActorIndex = t.Round.Dealer
		t.SendMaxHand(t.Round.Stage)
		return t.NextPlayer(1)
	case Showdown:
		t.Showdown()
		t.Round.Stage = End
		return t.MoveOn()
	case End:
		return t.ShowStatus()
	}
	return nil
}

func (t *Texas) NextPlayer(ignoreTimes int) error {
	breakForBet := false
	// Go to next one.
	t.Round.ActorIndex = t.Round.NextValidIndex(t.Round.ActorIndex)
	// ALL IN or last raiser.
	for t.Round.ActorIndex != t.Round.LastRaiser || ignoreTimes > 0 {
		if t.Round.ActorIndex == t.Round.LastRaiser {
			ignoreTimes--
		}
		if t.Players[t.Round.ActorIndex].Chip > 0 {
			breakForBet = true
			break
		}
		t.Round.ActorIndex = t.Round.NextValidIndex(t.Round.ActorIndex)
	}
	if t.Round.ActorIndex == t.Round.LastRaiser && !breakForBet {
		t.Round.Stage += 1
		return t.MoveOn()
	} else {
		return t.ShowStatus()
	}
}

func (t *Texas) getMaxAndCurrentUserIndex(userID int) (int64, int) {
	index := -1
	var max int64 = 0
	for i := 0; i < 10; i++ {
		if t.Round.UserState[i] == InGame && t.Players[i].UserID == userID {
			index = i
		}
		if t.Round.StageBets[i] > max {
			max = t.Round.StageBets[i]
		}
	}
	return max, index
}

func (t *Texas) getResultForFold() {
	for i := 0; i < 10; i++ {
		if t.Round.UserState[i] == InGame {
			// Only one
			t.Round.Earn[i] = t.Round.Pot
			break
		}
	}
}

func (e PlayerHands) Len() int {
	return len(e)
}

func (e PlayerHands) Swap(i, j int) {
	e[i], e[j] = e[j], e[i]
}

func (e PlayerHands) Less(i, j int) bool {
	if LessThanCardSet(e[i].Hand, e[j].Hand) {
		return LessThanCardSet(e[i].Hand, e[j].Hand)
	} else if !LessThanCardSet(e[j].Hand, e[i].Hand) {
		return e[i].Bets > e[j].Bets
	}
	return false
}

func (t *Texas) getResultForShowdown() {
	allHands := make(PlayerHands, 0)
	for i := 0; i < 10; i++ {
		if t.Round.UserState[i] == InGame {
			allHands = append(allHands, &PlayerHand{
				Hand:  t.Round.TopCards[i],
				Index: i,
				Bets:  t.Round.TotalBets[i],
			})
		}
	}
	sort.Sort(allHands)
	n := len(allHands)
	topHand := allHands[n-1].Hand
	winnerCount := 1
	for i := n - 2; i >= 0; i-- {
		if LessThanCardSet(allHands[i].Hand, topHand) {
			break
		}
		winnerCount += 1
	}
	rest := t.Round.Pot
	for i := n - 1; i >= 0; i-- {
		idx := allHands[i].Index
		if LessThanCardSet(allHands[i].Hand, topHand) {
			if rest > 0 {
				get := min(t.Round.TotalBets[idx], rest)
				rest -= get
				t.Round.Earn[idx] = get
			} else {
				break
			}
		} else {
			get := min(t.Round.TotalBets[idx]*int64(n), rest/int64(winnerCount))
			t.Round.Earn[idx] = get
			rest -= get
			winnerCount -= 1
		}
	}
}

func (t *Texas) Fold(userID int) error {
	_, index := t.getMaxAndCurrentUserIndex(userID)
	if index >= 0 {
		t.Round.UserState[index] = Fold
		if t.CountUserInGame() == 1 {
			t.getResultForFold()
			t.Round.Stage = End
			return t.MoveOn()
		}
		return t.NextPlayer(0)
	}
	return nil
}

func (t *Texas) Call(userID int) error {
	max, index := t.getMaxAndCurrentUserIndex(userID)
	if index >= 0 {
		if max == t.Round.StageBets[index] {
			return errors.New("You can only /check, /raise or /fold.")
		}
		t.MakeBet(index, (max - t.Round.StageBets[index]))
		return t.NextPlayer(0)
	}
	return nil
}

func (t *Texas) Check(userID int) error {
	max, index := t.getMaxAndCurrentUserIndex(userID)
	if index >= 0 {
		if max <= t.Round.StageBets[index] {
			return t.NextPlayer(0)
		} else {
			return errors.New("You can only /call, /raise or /fold.")
		}
	}
	return nil
}

func (t *Texas) Raise(userID int, amount int64) error {
	max, index := t.getMaxAndCurrentUserIndex(userID)
	if index >= 0 {
		if amount < 100 {
			return errors.New("Cannot /raise less than 100.")
		}
		delta := amount + (max - t.Round.StageBets[index])
		if t.Players[index].Chip <= delta {
			return errors.New("No enough chips for raising. /allin?")
		} else {
			t.MakeBet(index, delta)
			t.Round.LastRaiser = index
			return t.NextPlayer(0)
		}
	}
	return nil
}

func (t *Texas) GetOut(userID int) error {
	for i := 0; i < 10; i++ {
		if t.Round != nil && t.Players[i].UserID == userID {
			t.Round.UserState[i] = Out
			break
		}
	}
	return nil
}

func (t *Texas) AllIn(userID int) error {
	max, index := t.getMaxAndCurrentUserIndex(userID)
	if index >= 0 {
		all := t.Players[index].Chip + t.Round.StageBets[index]
		t.MakeBet(index, t.Players[index].Chip)
		if all > max {
			t.Round.LastRaiser = index
		}
		return t.NextPlayer(0)
	}
	return nil
}

func (t *Texas) Showdown() error {
	text := "= SHOWDOWN =\nCommunity Cards:"
	for i := 0; i < 5; i++ {
		text += " " + getPokerText(t.Round.CommunityCards[i])
	}
	text += "\n"
	count := 0
	for i := 0; i < 10; i++ {
		if t.Round.UserState[i] == InGame {
			count += 1
			text += fmt.Sprintf("[%d] %s(%d) - %s %s \\%s/\n", count,
				t.Players[i].DisplayName,
				t.Players[i].Chip,
				getPokerText(t.Round.PlayerCards[i][0]),
				getPokerText(t.Round.PlayerCards[i][1]),
				PokerHands[t.Round.TopCards[i].GetRank()])
		} else if t.Round.UserState[i] == Fold {
			count += 1
			text += fmt.Sprintf("[%d] %s(%d) - FOLD\n", count,
				t.Players[i].DisplayName,
				t.Players[i].Chip)
		}
	}
	t.getResultForShowdown()
	_, err := t.Bot.SendMessage(&SendMessageRequest{
		ChatID: t.ChatID,
		Text:   text,
	})
	return err
}

func (t *Texas) ShowStatus() error {
	text := "- " + StageNames[t.Round.Stage] + " - Pot: " +
		strconv.FormatInt(t.Round.Pot, 10) + "\nCommunity cards:"
	for i := 0; i < 5; i++ {
		if t.Round.CommunityCards[i] != nil {
			text += " " + getPokerText(t.Round.CommunityCards[i])
		} else {
			text += " ？"
		}
	}
	text += "\n"
	buttons := make([]*KeyboardButton, 0)
	if t.Round.Stage == End {
		count := 0
		for i := 0; i < 10; i++ {
			if t.Round.UserState[i] != Out {
				count++
				text += fmt.Sprintf("[%d] %s", count, t.Players[i].DisplayName)
				if t.Round.UserState[i] == Fold {
					text += " FOLD"
				} else if t.Round.Earn[i]-t.Round.TotalBets[i] > 0 {
					text += " WIN +" + strconv.FormatInt(t.Round.Earn[i]-t.Round.TotalBets[i], 10)
					t.Players[i].Chip += t.Round.Earn[i]
				} else {
					text += " LOSE"
					t.Players[i].Chip += t.Round.Earn[i]
				}
				text += " -> " + strconv.FormatInt(t.Players[i].Chip, 10) + " chips."
				if t.Players[i].Chip <= 0 {
					t.RemoveUser(&User{
						ID: t.Players[i].UserID,
					})
					text += "**GET OUT**"
				}
				text += "\n"
			}
		}
		buttons = append(buttons, &KeyboardButton{
			Text: "/start",
		}, &KeyboardButton{
			Text: "/getmoney",
		})
	} else {
		count := 0
		var max int64 = 0
		actor := t.Round.ActorIndex
		for i := 0; i < 10; i++ {
			if t.Round.UserState[i] != Out {
				count++
				if t.Round.StageBets[i] > max {
					max = t.Round.StageBets[i]
				}
				if i == t.Round.ActorIndex {
					text += "-> "
				}
				text += fmt.Sprintf("[%d] %s", count, t.Players[i].DisplayName)
				if t.Round.UserState[i] == Fold {
					text += " FOLD"
				} else {
					if t.Round.StageBets[i] > 0 {
						text += " +"
					} else {
						text += " "
					}
					text += strconv.FormatInt(t.Round.StageBets[i], 10)
					text += " / $" + strconv.FormatInt(t.Players[i].Chip, 10)
					if t.Players[i].Chip <= 0 {
						text += " *ALL IN*"
					}
				}
				if i == t.Round.Dealer {
					text += " (Dealer)"
				}
				text += "\n"
			}
		}
		text += fmt.Sprintf("Waiting %s (@%s)...",
			t.Players[actor].DisplayName, t.Players[actor].Username)
		if max <= t.Round.StageBets[actor] {
			buttons = append(buttons, &KeyboardButton{
				Text: "/check",
			})
		}
		if max > t.Round.StageBets[actor] &&
			(t.Players[actor].Chip > (max - t.Round.StageBets[actor])) {
			buttons = append(buttons, &KeyboardButton{
				Text: "/call",
			})
		}
		if t.Players[actor].Chip > (max - t.Round.StageBets[actor] + 200) {
			buttons = append(buttons, &KeyboardButton{
				Text: "/raise",
			})
		} else {
			buttons = append(buttons, &KeyboardButton{
				Text: "/allin",
			})
		}
		buttons = append(buttons, &KeyboardButton{
			Text: "/fold",
		})
	}
	if len(buttons) == 0 {
		_, err := t.Bot.SendMessage(&SendMessageRequest{
			ChatID: t.ChatID,
			Text:   text,
		})
		return err
	} else {
		_, err := t.Bot.SendMessage(&SendMessageRequest{
			ChatID: t.ChatID,
			Text:   text,
			ReplyMarkup: &ReplyKeyboardMarkup{
				Keyboard:        [][]*KeyboardButton{buttons},
				ResizeKeyboard:  true,
				OneTimeKeyboard: true,
				// /start should not be selective
				Selective: len(buttons) > 2,
			},
		})
		return err
	}
}

func (r *Round) NextValidIndex(index int) int {
	for i := 1; i <= 10; i++ {
		if r.UserState[(index+i)%10] == InGame {
			return (index + i) % 10
		}
	}
	panic("NextValidIndex runs in an empty desk.")
}

func (t *Texas) MakeBet(index int, amount int64) {
	amount = min(t.Players[index].Chip, amount)
	t.Players[index].Chip -= amount
	t.Round.StageBets[index] += amount
	t.Round.TotalBets[index] += amount
	t.Round.Pot += amount
}
